package commands

import (
	EstructuraExt2 "MIA_2S2025_P1_202307820/EstructuraExt2"
	session "MIA_2S2025_P1_202307820/Session"
	structuraArchivoCarpeta "MIA_2S2025_P1_202307820/estructuraArchivoCarpeta"
	"MIA_2S2025_P1_202307820/utilities"
	"fmt"
	"os"
	"strings"
)

func CHGRP(args []string) {
	// verificar que root este logueado
	if !session.IsSessionActive() {
		fmt.Println("Error: Debe iniciar sesión para usar este comando")
		fmt.Println("Use: login -user=<usuario> -pass=<contraseña> -id=<partition_id>")
		return
	}

	sessionInfo := session.GetCurrentSession()

	if sessionInfo.Username != "root" {
		fmt.Printf("Error: Solo el usuario root puede cambiar grupos\n")
		fmt.Printf("Usuario actual: %s\n", sessionInfo.Username)
		return
	}

	// parsear parámetros
	params := utilities.ParseParameters(args)

	usarname := strings.Trim(params["user"], "\"")
	if usarname == "" {
		fmt.Println("Error: Falta el parámetro obligatorio -user")
		fmt.Println("Uso: chgrp -user=<nombre_usuario> -grp=<nombre_grupo>")
		return
	}

	if usarname == "root" {
		fmt.Println("Error: No se puede cambiar el grupo del usuario 'root'")
		return
	}

	groupName := strings.Trim(params["grp"], "\"")
	if groupName == "" {
		fmt.Println("Error: Debe especificar el nombre del grupo")
		fmt.Println("Uso: chgrp -user=<nombre_usuario> -grp=<nombre_grupo>")
		return
	}

	// obtener la partición montada
	mountedPartition, exists := GetMountedPartition(sessionInfo.PartitionID)
	if !exists {
		fmt.Printf("Error: La partición '%s' no está montada\n", sessionInfo.PartitionID)
		return
	}

	err := CambiarGrupoUsuario(mountedPartition, usarname, groupName)
	if err != nil {
		fmt.Printf("Error cambiando grupo: %s\n", err)
		return
	}

	fmt.Printf("Usuario '%s' ahora pertenece al grupo '%s'\n", usarname, groupName)
}

func CambiarGrupoUsuario(partition MountedPartition, username string, groupName string) error {
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

	// Leer el sistema EXT2 de la partición
	ext2fs, err := EstructuraExt2.ReadEXT2FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return fmt.Errorf("error leyendo sistema EXT2: %s", err)
	}

	if ext2fs.SuperBlock.S_magic != structuraArchivoCarpeta.EXT2_MAGIC {
		return fmt.Errorf("el sistema de archivos no está formateado como EXT2")
	}

	// BUSCAR EL ARCHIVO users.txt
	usersInodeNum, err := findFileInRootDirectory(file, ext2fs, "users.txt")
	if err != nil {
		return fmt.Errorf("error buscando archivo users.txt: %s", err)
	}

	// Leer el inodo de users.txt
	usersInode := &structuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (usersInodeNum * ext2fs.SuperBlock.S_inode_s))
	err = utilities.ReadStruct(file, inodeOffset, usersInode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo de users.txt: %s", err)
	}

	// LEER CONTENIDO ACTUAL DEL ARCHIVO users.txt
	currentContent, err := readFileContentFromBlocks(file, ext2fs, usersInode)
	if err != nil {
		return fmt.Errorf("error leyendo contenido de users.txt: %s", err)
	}

	fmt.Printf("DEBUG: Contenido actual de users.txt:\n%s\n", currentContent)

	// VERIFICAR QUE EL USUARIO EXISTE
	userLine, userExists := findUserInContent(currentContent, username)
	if !userExists {
		return fmt.Errorf("el usuario '%s' no existe", username)
	}

	// VERIFICAR QUE EL NUEVO GRUPO EXISTE Y ESTÁ ACTIVO
	if !groupExistsAndActive(currentContent, groupName) {
		return fmt.Errorf("el grupo '%s' no existe o está eliminado", groupName)
	}

	// CAMBIAR EL GRUPO DEL USUARIO
	newContent, err := changeUserGroup(currentContent, groupName, username, userLine)
	if err != nil {
		return fmt.Errorf("error cambiando grupo del usuario: %s", err)
	}

	// ESCRIBIR EL CONTENIDO ACTUALIZADO
	err = writeContentToFileUser(file, ext2fs, usersInode, newContent, usersInodeNum)
	if err != nil {
		return fmt.Errorf("error escribiendo contenido actualizado: %s", err)
	}

	fmt.Printf("Grupo del usuario '%s' cambiado exitosamente de '%s' a '%s'\n",
		username, extractGroupFromUserLine(userLine), groupName)

	return nil
}

// FUNCIÓN PARA BUSCAR UN USUARIO EN EL CONTENIDO
func findUserInContent(content, username string) (string, bool) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) >= 4 {
			// Formato: ID,U,GRUPO,USUARIO,PASSWORD
			if parts[1] == "U" && parts[3] == username && parts[0] != "0" {
				return line, true // Usuario encontrado y no está eliminado
			}
		}
	}
	return "", false
}

// FUNCIÓN PARA CAMBIAR EL GRUPO DE UN USUARIO
func changeUserGroup(content, newGroup, username, oldUserLine string) (string, error) {
	lines := strings.Split(content, "\n")
	var newLines []string

	for _, line := range lines {
		if strings.TrimSpace(line) == strings.TrimSpace(oldUserLine) {
			// Esta es la línea del usuario que queremos cambiar
			parts := strings.Split(line, ",")
			if len(parts) >= 5 {
				// Formato: ID,U,USUARIO,GRUPO_VIEJO,PASSWORD
				oldGroup := parts[3]
				newUserLine := fmt.Sprintf("%s,U,%s,%s,%s", parts[0], newGroup, username, parts[4])
				newLines = append(newLines, newUserLine)

				fmt.Printf("Cambiando usuario: %s\n", username)
				fmt.Printf("   Grupo anterior: %s\n", oldGroup)
				fmt.Printf("   Grupo nuevo: %s\n", newGroup)
				fmt.Printf("   Línea anterior: %s\n", line)
				fmt.Printf("   Línea nueva: %s\n", newUserLine)
			} else {
				return "", fmt.Errorf("formato de línea de usuario inválido: %s", line)
			}
		} else {
			newLines = append(newLines, line)
		}
	}

	return strings.Join(newLines, "\n"), nil
}

// FUNCIÓN PARA EXTRAER EL GRUPO DE UNA LÍNEA DE USUARIO
func extractGroupFromUserLine(userLine string) string {
	parts := strings.Split(userLine, ",")
	if len(parts) >= 4 {
		return parts[3] // El grupo está en la posición 3
	}
	return "desconocido"
}
