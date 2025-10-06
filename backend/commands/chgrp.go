package commandsP2

import (
	bloques "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	session "MIA_2S2025_P2_202307820/Session"
	structuraArchivoCarpeta "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
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
	params := utilitiesP2.ParseParameters(args)

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

	fsType, err := getFileSystemType(mountedPartition)
	if err != nil {
		fmt.Printf("Error obteniendo tipo de sistema de archivos: %s\n", err)
		return
	}

	if fsType == structuraArchivoCarpeta.EXT2_FILESYSTEM_TYPE {
		err = CambiarGrupoUsuario(mountedPartition, usarname, groupName)
	} else if fsType == structuraArchivoCarpeta.EXT3_FILESYSTEM_TYPE {
		err = CambiarGrupoUsuarioEXT3(mountedPartition, usarname, groupName)
	} else {
		fmt.Printf("Error: El sistema de archivos '%d' no es compatible con este comando\n", fsType)
		return
	}

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
	err = utilitiesP2.ReadStruct(file, inodeOffset, usersInode)
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

func CambiarGrupoUsuarioEXT3(partition MountedPartition, username string, groupName string) error {
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

	// Leer el sistema EXT3 de la partición
	ext3fs, err := EstructuraExt3P2.ReadEXT3FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return fmt.Errorf("error leyendo sistema EXT3: %s", err)
	}

	if ext3fs.SuperBlock.S_magic != structuraArchivoCarpeta.EXT2_MAGIC {
		return fmt.Errorf("el sistema de archivos no está formateado como EXT3")
	}

	// BUSCAR EL ARCHIVO users.txt
	usersInodeNum, err := findFileInRootDirectoryEXT3(file, ext3fs, "users.txt")
	if err != nil {
		return fmt.Errorf("error buscando archivo users.txt: %s", err)
	}

	// Leer el inodo de users.txt
	usersInode := &structuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (usersInodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilitiesP2.ReadStruct(file, inodeOffset, usersInode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo de users.txt: %s", err)
	}

	// LEER CONTENIDO ACTUAL DEL ARCHIVO users.txt
	currentContent, err := readFileContentFromBlocksEXT3(file, ext3fs, usersInode)
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
	err = writeContentToFileUserEXT3(file, ext3fs, usersInode, newContent, usersInodeNum)
	if err != nil {
		return fmt.Errorf("error escribiendo contenido actualizado: %s", err)
	}

	LogEXT3Operation(EstructuraExt3P2.OPERATION_CHGRP, "/users.txt", fmt.Sprintf("Usuario %s cambiado al grupo %s", username, groupName))

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

func writeContentToFileUserEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, inode *structuraArchivoCarpeta.Inodo, content string, inodeNum int32) error {
	contentBytes := []byte(content)
	contentSize := int32(len(contentBytes))

	fmt.Printf("DEBUG: Escribiendo %d bytes al archivo users.txt\n", contentSize)

	blocksNeed := (contentSize + 63) / 64 // Cada bloque es de 64 bytes

	blocks := inode.GetIblock()
	blocckUsed := int32(0)
	for i := 0; i < 12; i++ {
		if blocks[i] != -1 {
			blocckUsed++
		}
	}

	if blocksNeed > blocckUsed {
		for i := int(blocckUsed); i < 12 && i < int(blocksNeed); i++ {
			newBlockNum, err := findFreeBlockEXT3(file, ext3fs)
			if err != nil {
				return fmt.Errorf("no hay bloques libres: %s", err)
			}
			inode.SetIblock(i, newBlockNum)
			err = markBlockAsUsedEXT3For(file, ext3fs, newBlockNum)
			if err != nil {
				return fmt.Errorf("error marcando bloque %d como usado: %s", newBlockNum, err)
			}
			ext3fs.SuperBlock.S_free_blocks_count--
			fmt.Printf("bloque adicional %d asignado al inodo %d\n", newBlockNum, i)
		}
	}

	var apuntador *bloques.BloqueApuntador
	if blocksNeed > 12 {
		if inode.GetIblock()[12] == -1 {
			indirectBlockNum := inode.GetIblock()[12]
			apuntador = bloques.NewBloqueApuntador()
			blockOffset := int64(ext3fs.SuperBlock.S_block_s + (indirectBlockNum * ext3fs.SuperBlock.S_block_s))
			err := utilitiesP2.ReadStruct(file, blockOffset, apuntador)

			if err != nil {
				return fmt.Errorf("error leyendo bloque de apuntadores indirectos: %s", err)
			}
		} else {
			apuntador = bloques.NewBloqueApuntador()
			indirectBlockNum, err := findFreeBlockEXT3(file, ext3fs)
			if err != nil {
				return fmt.Errorf("no hay bloques libres para bloque de apuntadores: %s", err)
			}
			inode.SetIblock(12, indirectBlockNum)
			err = markBlockAsUsedEXT3For(file, ext3fs, indirectBlockNum)
			if err != nil {
				return fmt.Errorf("error marcando bloque de apuntadores como usado: %s", err)
			}
			ext3fs.SuperBlock.S_free_blocks_count--
		}

		for i := 12; i < int(blocksNeed) && (i-12) < 16; i++ {
			if apuntador.B_pointers[i-12] == -1 {
				blockNum, err := findFreeBlockEXT3(file, ext3fs)
				if err != nil {
					return fmt.Errorf("no se pudo encontrar un bloque libre: %s", err)
				}
				apuntador.B_pointers[i-12] = blockNum
				err = markBlockAsUsedEXT3For(file, ext3fs, blockNum)
				if err != nil {
					return fmt.Errorf("error marcando bloque como usado: %s", err)
				}
				ext3fs.SuperBlock.S_free_blocks_count--
			}
		}

		blockOffset := int64(ext3fs.SuperBlock.S_block_start + (inode.GetIblock()[12] * ext3fs.SuperBlock.S_block_s))
		err := utilitiesP2.WriteStruct(file, blockOffset, apuntador)
		if err != nil {
			return fmt.Errorf("error escribiendo bloque de apuntadores: %s", err)
		}
		fmt.Printf("[DEBUG] Bloque Apuntador Simple (bloque %d): %v\n", inode.GetIblock()[12], apuntador.B_pointers)
	}

	var dobleApuntador *bloques.BloqueApuntador // CORREGIDO: Declarar fuera del if
	if blocksNeed > 28 {
		dobleApuntador = bloques.NewBloqueApuntador()
		if inode.GetIblock()[13] != -1 {
			dobleBlockNum := inode.GetIblock()[13]
			dobleOffset := int64(ext3fs.SuperBlock.S_block_start + (dobleBlockNum * ext3fs.SuperBlock.S_block_s))
			utilitiesP2.ReadStruct(file, dobleOffset, dobleApuntador)
		} else {
			dobleBlockNum, err := findFreeBlockEXT3(file, ext3fs)
			if err != nil {
				return fmt.Errorf("no hay bloques libres para bloque de apuntadores doble: %s", err)
			}
			inode.SetIblock(13, dobleBlockNum)
			err = markBlockAsUsedEXT3For(file, ext3fs, dobleBlockNum)
			if err != nil {
				return fmt.Errorf("error marcando bloque de apuntadores doble como usado: %s", err)
			}
			ext3fs.SuperBlock.S_free_blocks_count--
		}

		// CORREGIDO: Lógica de asignación de bloques dobles
		for i := int32(28); i < blocksNeed && i < 284; i++ {
			hijoIndex := (i - 28) / 16
			hijoOffset := (i - 28) % 16

			var hijoNum int32
			var hijo *bloques.BloqueApuntador

			// Crear o leer el hijo
			if dobleApuntador.B_pointers[hijoIndex] == -1 {
				hijo = bloques.NewBloqueApuntador()
				hijoNum, err := findFreeBlockEXT3(file, ext3fs)
				if err != nil {
					return fmt.Errorf("no hay bloques libres para bloque de apuntadores doble hijo: %s", err)
				}
				dobleApuntador.B_pointers[hijoIndex] = hijoNum
				err = markBlockAsUsedEXT3For(file, ext3fs, hijoNum) // CORREGIDO: Marcar bloque hijo como usado
				if err != nil {
					return fmt.Errorf("error marcando bloque hijo como usado: %s", err)
				}
				ext3fs.SuperBlock.S_free_blocks_count--
			} else {
				hijoNum = dobleApuntador.B_pointers[hijoIndex]
				hijo = bloques.NewBloqueApuntador()
				hijoOffsetDisk := int64(ext3fs.SuperBlock.S_block_start + (hijoNum * ext3fs.SuperBlock.S_block_s))
				utilitiesP2.ReadStruct(file, hijoOffsetDisk, hijo)
			}

			// Asignar bloque de datos si no existe
			if hijo.B_pointers[hijoOffset] == -1 {
				blockNum, err := findFreeBlockEXT3(file, ext3fs)
				if err != nil {
					return fmt.Errorf("no se pudo encontrar un bloque libre: %s", err)
				}
				hijo.B_pointers[hijoOffset] = blockNum
				err = markBlockAsUsedEXT3For(file, ext3fs, blockNum)
				if err != nil {
					return fmt.Errorf("error marcando bloque como usado: %s", err)
				}
				ext3fs.SuperBlock.S_free_blocks_count--
			}

			// Escribir el bloque hijo
			hijoOffsetDisk := int64(ext3fs.SuperBlock.S_block_start + (hijoNum * ext3fs.SuperBlock.S_block_s))
			utilitiesP2.WriteStruct(file, hijoOffsetDisk, hijo)
		}

		// Escribir el bloque doble principal
		dobleBlockNum := inode.GetIblock()[13]
		dobleOffset := int64(ext3fs.SuperBlock.S_block_start + (dobleBlockNum * ext3fs.SuperBlock.S_block_s))
		err := utilitiesP2.WriteStruct(file, dobleOffset, dobleApuntador)
		if err != nil {
			return fmt.Errorf("error escribiendo bloque de apuntadores doble: %s", err)
		}
		fmt.Printf("[DEBUG] Bloque Apuntador Doble (bloque %d): %v\n", dobleBlockNum, dobleApuntador.B_pointers)
	}

	// CORREGIDO: Función getBlockNumber para manejar todos los tipos de bloques
	getBlockNumber := func(index int32) int32 {
		if index < 12 {
			return inode.GetIblock()[index]
		} else if index < 28 {
			if apuntador != nil {
				return apuntador.B_pointers[index-12]
			}
		} else if index < 284 {
			if dobleApuntador != nil {
				hijoIndex := (index - 28) / 16
				hijoOffset := (index - 28) % 16
				if dobleApuntador.B_pointers[hijoIndex] != -1 {
					// Leer el bloque hijo
					hijo := bloques.NewBloqueApuntador()
					hijoNum := dobleApuntador.B_pointers[hijoIndex]
					hijoOffsetDisk := int64(ext3fs.SuperBlock.S_block_start + (hijoNum * ext3fs.SuperBlock.S_block_s))
					utilitiesP2.ReadStruct(file, hijoOffsetDisk, hijo)
					return hijo.B_pointers[hijoOffset]
				}
			}
		}
		return -1
	}

	// ESCRIBIR CONTENIDO EN TODOS LOS BLOQUES NECESARIOS
	bytesWritten := int32(0)
	for blockIndex := 0; blockIndex < int(blocksNeed); blockIndex++ {
		blockNum := getBlockNumber(int32(blockIndex)) // CORREGIDO: Usar la función helper
		if blockNum == -1 {
			return fmt.Errorf("bloque %d no asignado correctamente", blockIndex)
		}

		remainingBytes := contentSize - bytesWritten
		bytesToWrite := int32(64)
		if remainingBytes < bytesToWrite {
			bytesToWrite = remainingBytes
		}

		fileBlock := &bloques.BloqueArchivo{}
		for i := 0; i < 64; i++ {
			fileBlock.B_content[i] = 0
		}
		if bytesToWrite > 0 {
			copy(fileBlock.B_content[:bytesToWrite], contentBytes[bytesWritten:bytesWritten+bytesToWrite])
		}

		blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))
		err := utilitiesP2.WriteStruct(file, blockOffset, fileBlock)
		if err != nil {
			return fmt.Errorf("error escribiendo bloque %d: %s", blockNum, err)
		}
		bytesWritten += bytesToWrite
		fmt.Printf("DEBUG: Bloque %d escrito: %d bytes (total: %d)\n", blockIndex, bytesToWrite, bytesWritten)
	}

	// LIMPIAR BLOQUES NO USADOS (si el archivo se redujo)
	for i := int(blocksNeed); i < 12; i++ {
		if inode.GetIblock()[i] != -1 {
			oldBlockNum := inode.GetIblock()[i]
			emptyBlock := &bloques.BloqueArchivo{}
			blockOffset := int64(ext3fs.SuperBlock.S_block_start + (oldBlockNum * ext3fs.SuperBlock.S_block_s))
			utilitiesP2.WriteStruct(file, blockOffset, emptyBlock)
			markBlockAsFreeEXT3For(file, ext3fs, oldBlockNum)
			inode.SetIblock(i, -1)
			ext3fs.SuperBlock.S_free_blocks_count++
			fmt.Printf("🗑️  Bloque %d liberado de posición %d\n", oldBlockNum, i)
		}
	}

	// Actualizar el superbloque
	err := utilitiesP2.WriteStruct(file, int64(ext3fs.StartOffset), ext3fs.SuperBlock)
	if err != nil {
		return fmt.Errorf("error actualizando superbloque: %s", err)
	}

	// ACTUALIZAR EL INODO
	inode.SetSize(contentSize)
	inode.UpdateModificationTime()
	inodeOffset := int64(ext3fs.SuperBlock.S_inode_start + (inodeNum * ext3fs.SuperBlock.S_inode_s))
	err = utilitiesP2.WriteStruct(file, inodeOffset, inode)
	if err != nil {
		return fmt.Errorf("error actualizando inodo: %s", err)
	}

	return nil

}

func updateBlockBitmapEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, blockNum int32, used bool) error {
	byteIndex := blockNum / 8
	bitIndex := blockNum % 8

	_, err := file.Seek(int64(ext3fs.SuperBlock.S_bm_block_start)+int64(byteIndex), 0)
	if err != nil {
		return fmt.Errorf("error seeking block bitmap: %s", err)
	}

	var bitmapByte [1]byte
	_, err = file.Read(bitmapByte[:])
	if err != nil {
		return err
	}

	mask := byte(1 << (7 - bitIndex))
	if used {
		bitmapByte[0] |= mask // Marcar como usado
	} else {
		bitmapByte[0] &^= mask // Marcar como libre
	}

	_, err = file.Seek(int64(ext3fs.SuperBlock.S_bm_block_start)+int64(byteIndex), 0)
	if err != nil {
		return fmt.Errorf("error seeking block bitmap for write: %s", err)
	}

	_, err = file.Write(bitmapByte[:])
	return err
}

func markBlockAsUsedEXT3For(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, blockNum int32) error {
	return updateBlockBitmapEXT3(file, ext3fs, blockNum, true)
}

func markBlockAsFreeEXT3For(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, blockNum int32) error {
	return updateBlockBitmapEXT3(file, ext3fs, blockNum, false)
}
