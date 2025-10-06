package commandsP2

import (
	EstructuraExt2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt3 "MIA_2S2025_P2_202307820/EstructuraExt3"
	session "MIA_2S2025_P2_202307820/Session"
	estructuraArchivoCarpeta "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"strings"
)

// CAT implementa el comando para mostrar el contenido de archivos
func CAT(args []string) {
	// Verificar que hay una sesión activa
	if !session.IsSessionActive() {
		fmt.Println("Error: Debe iniciar sesión para usar este comando")
		fmt.Println("Use: login -user=<usuario> -pass=<contraseña> -id=<partition_id>")
		return
	}

	// Parsear parámetros usando utilities
	params := utilitiesP2.ParseParameters(args)

	// Buscar archivos (file1, file2, file3, etc.)
	var files []string
	fileIndex := 1

	for {
		fileKey := fmt.Sprintf("file%d", fileIndex)
		if filePath, exists := params[fileKey]; exists {
			// Limpiar comillas si las tiene
			filePath = strings.Trim(filePath, "\"")
			files = append(files, filePath)
			fileIndex++
		} else {
			break
		}
	}

	// Verificar que se especificó al menos un archivo
	if len(files) == 0 {
		fmt.Println("Error: Debe especificar al menos un archivo usando -file1=<ruta>")
		fmt.Println("Ejemplo: cat -file1=/home/archivo.txt")
		return
	}

	// Obtener información de la sesión
	sessionInfo := session.GetCurrentSession()

	// Obtener la partición montada de la sesión
	mountedPartition, exists := GetMountedPartition(sessionInfo.PartitionID)
	if !exists {
		fmt.Printf("Error: La partición '%s' no está montada\n", sessionInfo.PartitionID)
		return
	}

	fsType := int32(2) // Por defecto EXT2
	{
		file, err := os.Open(mountedPartition.Path)
		if err == nil {
			sb := &estructuraArchivoCarpeta.SuperBloque{}
			utilitiesP2.ReadStruct(file, int64(mountedPartition.Start), sb)
			fsType = sb.S_filesystem_type
			file.Close()
		}
	}

	// Procesar cada archivo
	fmt.Printf("=== CONTENIDO DE ARCHIVOS ===\n\n")

	for i, filePath := range files {
		if i > 0 {
			fmt.Println() // Separador entre archivos
		}

		fmt.Printf("--- Archivo %d: %s ---\n", i+1, filePath)

		// Leer y mostrar el contenido del archivo
		var content string
		var err error

		if fsType == estructuraArchivoCarpeta.EXT3_FILESYSTEM_TYPE {
			content, err = readFileFromEXT3(mountedPartition, filePath, sessionInfo)
		} else {
			content, err = readFileContent(mountedPartition, filePath, sessionInfo.UID, sessionInfo.GID)
		}

		if err != nil {
			fmt.Printf("Error: %s\n", err)
			continue
		}

		// Mostrar el contenido
		if content == "" {
			fmt.Println("(archivo vacío)")
		} else {
			fmt.Print(content)
			// Agregar salto de línea si el archivo no termina en uno
			if !strings.HasSuffix(content, "\n") {
				fmt.Println()
			}
		}
	}

	fmt.Printf("\n=== FIN DE CONTENIDO ===\n")
	fmt.Printf("Total de archivos procesados: %d\n", len(files))
}

func readFileFromEXT3(partition MountedPartition, filePath string, sessionInfo *session.UserSession) (string, error) {
	// Abrir el archivo del disco
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return "", fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

	// Leer el sistema EXT3 de la partición
	ext3fs, err := EstructuraExt3.ReadEXT3FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return "", fmt.Errorf("error leyendo sistema EXT3: %s", err)
	}

	// Verificar que el sistema esté formateado
	if ext3fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return "", fmt.Errorf("la partición no tiene formato EXT3 válido")
	}

	// Leer el archivo usando EXT3
	content, err := ext3fs.LeerArchivo(filePath)
	if err != nil {
		return "", fmt.Errorf("error leyendo archivo '%s': %s", filePath, err)
	}

	// Registrar la operación en el journal
	LogEXT3Operation(EstructuraExt3.OPERATION_CAT, filePath, fmt.Sprintf("%d", len(content)))

	return content, nil
}

// readFileFromEXT2 lee el contenido real de un archivo desde el sistema EXT2
func readFileFromEXT2(partition MountedPartition, filePath string) (string, error) {
	// IMPLEMENTACIÓN REAL usando las funciones auxiliares

	// Abrir el archivo del disco
	file, err := os.OpenFile(partition.Path, os.O_RDONLY, 0666)
	if err != nil {
		return "", fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

	// Leer el sistema EXT2 de la partición
	ext2fs, err := EstructuraExt2.ReadEXT2FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return "", fmt.Errorf("error leyendo sistema EXT2: %s", err)
	}

	// Verificar que el sistema esté formateado
	if ext2fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return "", fmt.Errorf("la partición no tiene formato EXT2 válido")
	}

	fileInodeNum, err := findFileByPath(file, ext2fs, filePath)
	if err != nil {
		return "", fmt.Errorf("archivo no encontrado: %s", err)
	}

	// Leer el inodo del archivo
	fileInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (fileInodeNum * ext2fs.SuperBlock.S_inode_s))
	err = utilitiesP2.ReadStruct(file, inodeOffset, fileInode)
	if err != nil {
		return "", fmt.Errorf("error leyendo inodo del archivo: %s", err)
	}

	// Leer el contenido del archivo desde sus bloques
	content, err := readFileContentFromBlocks(file, ext2fs, fileInode)
	if err != nil {
		return "", fmt.Errorf("error leyendo contenido del archivo: %s", err)
	}

	return content, nil
}

// FUNCIÓN CLAVE: findFileByPath busca un archivo por su ruta completa
func findFileByPath(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, filePath string) (int32, error) {
	if filePath == "/" {
		return 0, nil // Inodo raíz
	}

	// Dividir el path en componentes
	pathComponents := strings.Split(strings.Trim(filePath, "/"), "/")
	currentInodeNum := int32(0) // Empezar desde la raíz

	// Navegar por cada componente del path
	for i, component := range pathComponents {
		if component == "" {
			continue
		}

		// Buscar el componente en el directorio actual
		foundInodeNum, err := findFileInDirectory(file, ext2fs, currentInodeNum, component)
		if err != nil {
			return -1, fmt.Errorf("error buscando componente '%s': %s", component, err)
		}

		if foundInodeNum == -1 {
			// No encontrado
			if i == len(pathComponents)-1 {
				return -1, fmt.Errorf("archivo '%s' no encontrado", component)
			} else {
				currentPath := "/" + strings.Join(pathComponents[:i+1], "/")
				return -1, fmt.Errorf("directorio '%s' no encontrado", currentPath)
			}
		}

		// Si no es el último componente, verificar que es un directorio
		if i < len(pathComponents)-1 {
			foundInode := &estructuraArchivoCarpeta.Inodo{}
			inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (foundInodeNum * ext2fs.SuperBlock.S_inode_s))
			err = utilitiesP2.ReadStruct(file, inodeOffset, foundInode)
			if err != nil {
				return -1, fmt.Errorf("error leyendo inodo: %s", err)
			}

			if !foundInode.IsDirectory() {
				currentPath := "/" + strings.Join(pathComponents[:i+1], "/")
				return -1, fmt.Errorf("'%s' no es un directorio", currentPath)
			}
		}

		currentInodeNum = foundInodeNum
	}

	return currentInodeNum, nil
}

// readFileContent lee el contenido de un archivo desde el sistema EXT2
func readFileContent(partition MountedPartition, filePath string, userUID, userGID int32) (string, error) {
	// Validar que la ruta sea absoluta
	if !strings.HasPrefix(filePath, "/") {
		return "", fmt.Errorf("la ruta '%s' debe ser absoluta (comenzar con /)", filePath)
	}

	// Verificar que el archivo existe y tiene permisos de lectura
	hasReadPermission, err := checkFileReadPermission(partition, filePath, userUID, userGID)
	if err != nil {
		return "", err
	}

	if !hasReadPermission {
		return "", fmt.Errorf("no tiene permisos de lectura para el archivo '%s'", filePath)
	}

	// Leer el contenido real del archivo desde el sistema EXT2
	content, err := readFileFromEXT2(partition, filePath)
	if err != nil {
		return "", fmt.Errorf("error leyendo archivo '%s': %s", filePath, err)
	}

	return content, nil
}

// checkFileReadPermission verifica si el usuario tiene permisos de lectura sobre un archivo
func checkFileReadPermission(partition MountedPartition, filePath string, userUID, userGID int32) (bool, error) {

	// Root (UID 1 según especificación) puede leer cualquier archivo
	if userUID == 1 {
		return true, nil
	}

	// Abrir el archivo del disco
	file, err := os.OpenFile(partition.Path, os.O_RDONLY, 0666)
	if err != nil {
		return false, fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

	// Leer el sistema EXT2 de la partición
	ext2fs, err := EstructuraExt2.ReadEXT2FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return false, fmt.Errorf("error leyendo sistema EXT2: %s", err)
	}

	// Verificar que el sistema esté formateado
	if ext2fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return false, fmt.Errorf("la partición no tiene formato EXT2 válido")
	}

	// Obtener solo el nombre del archivo (para búsqueda en directorio raíz)
	pathComponents := strings.Split(strings.Trim(filePath, "/"), "/")
	filename := pathComponents[len(pathComponents)-1]

	// Buscar el archivo en el directorio raíz
	fileInodeNum, err := findFileInRootDirectory(file, ext2fs, filename)
	if err != nil {
		return false, fmt.Errorf("archivo no encontrado: %s", err)
	}

	// Leer el inodo del archivo para obtener permisos
	fileInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (fileInodeNum * ext2fs.SuperBlock.S_inode_s))
	err = utilitiesP2.ReadStruct(file, inodeOffset, fileInode)
	if err != nil {
		return false, fmt.Errorf("error leyendo inodo del archivo: %s", err)
	}

	// Obtener información del archivo usando métodos correctos
	fileUID := fileInode.GetIuid()
	fileGID := fileInode.GetIgid()
	permissions := fileInode.GetIperm()

	// 1. Si el usuario es el propietario del archivo
	if userUID == fileUID {
		// Verificar permiso de lectura del propietario
		// Los permisos en Unix son: rwx = 4+2+1, donde r(read)=4
		ownerPermission := permissions[0]
		hasReadPermission := (ownerPermission & 4) != 0
		return hasReadPermission, nil
	}

	// 2. Si el usuario pertenece al mismo grupo que el archivo
	if userGID == fileGID {
		// Verificar permiso de lectura del grupo
		groupPermission := permissions[1]
		hasReadPermission := (groupPermission & 4) != 0
		return hasReadPermission, nil
	}

	// 3. Permisos para otros usuarios
	// Verificar permiso de lectura de otros
	otherPermission := permissions[2]
	hasReadPermission := (otherPermission & 4) != 0
	return hasReadPermission, nil
}
