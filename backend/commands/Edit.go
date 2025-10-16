package commandsP2

import (
	bloques "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	sessionP2 "MIA_2S2025_P2_202307820/Session"
	estructuraArchivoCarpeta "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// EDIT implementa el comando para editar el contenido de un archivo
func EDIT(args []string) {
	// Verificar que hay una sesión activa
	if !sessionP2.IsSessionActive() {
		fmt.Printf("Error: Debe iniciar sesión para usar este comando\n")
		fmt.Printf("Use: login -user=<usuario> -pass=<contraseña> -id=<partition_id>\n")
		return
	}

	// Parsear parámetros
	params := utilitiesP2.ParseParameters(args)
	path := strings.Trim(params["path"], "\"")
	content := strings.Trim(params["contenido"], "\"")

	// Validar parámetros obligatorios
	if path == "" || content == "" {
		fmt.Printf("Error: Debe especificar la ruta del archivo a editar y el contenido\n")
		fmt.Printf("Uso: edit -path=<ruta_archivo> -contenido=<ruta_contenido>\n")
		return
	}

	// Validar que la ruta sea absoluta
	if !strings.HasPrefix(path, "/") {
		fmt.Printf("Error: La ruta del archivo debe ser absoluta y comenzar con '/'")
		return
	}

	// Obtener información de la sesión
	sessionInfo := sessionP2.GetCurrentSession()
	if sessionInfo == nil {
		fmt.Printf("Error: No se pudo obtener la información de la sesión\n")
		return
	}

	// Obtener la partición montada
	mountedPartition, exists := GetMountedPartition(sessionInfo.PartitionID)
	if !exists {
		fmt.Printf("Error: La partición '%s' no está montada\n", sessionInfo.PartitionID)
		return
	}

	// Leer el contenido del archivo externo
	contentBytes, err := ioutil.ReadFile(content)
	if err != nil {
		fmt.Printf("Error al leer el archivo de contenido '%s': %v\n", content, err)
		return
	}

	// Editar el archivo en la partición
	err = EditFile(mountedPartition, path, string(contentBytes), sessionInfo)
	if err != nil {
		fmt.Printf("Error al editar el archivo: %v\n", err)
		return
	}

	fmt.Printf("Archivo '%s' editado exitosamente\n", path)
}

// EditFile edita el contenido de un archivo existente en el sistema EXT2/EXT3
func EditFile(partition MountedPartition, filePath string, newContent string, sessionInfo *sessionP2.UserSession) error {
	// Abrir el archivo del disco
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("no se pudo abrir el disco: %v", err)
	}
	defer file.Close()

	superBlock := &estructuraArchivoCarpeta.SuperBloque{}
	err = utilitiesP2.ReadStruct(file, int64(partition.Start), superBlock)
	if err != nil {
		return fmt.Errorf("no se pudo leer el superbloque: %v", err)
	}

	// Determinar el tipo de sistema de archivos
	var fileSystem int32
	switch superBlock.S_filesystem_type {
	case 2:
		fileSystem = 2
	case 3:
		fileSystem = 3
	default:
		return fmt.Errorf("sistema de archivos no soportado: %s", partition.Type)
	}

	// Extraer el directorio padre y nombre del archivo
	parentPath := filepath.Dir(filePath)
	if parentPath == "/" {
		parentPath = ""
	}
	fileName := filepath.Base(filePath)

	switch fileSystem {
	case 2:
		// Leer el sistema EXT2
		ext2fs, err := EstructuraExt2.ReadEXT2FileSystem(partition.Path, partition.Start)
		if err != nil {
			return fmt.Errorf("error al leer el sistema EXT2: %v", err)
		}

		// Buscar el directorio padre
		parentInodeNum, err := findDirectoryInEXT2(file, ext2fs, parentPath)
		if err != nil {
			return fmt.Errorf("error al buscar el directorio padre: %v", err)
		}

		// Leer el inodo del directorio padre
		parentInode := &estructuraArchivoCarpeta.Inodo{}
		err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+parentInodeNum*ext2fs.SuperBlock.S_inode_s), parentInode)
		if err != nil {
			return fmt.Errorf("error al leer el inodo del directorio padre: %v", err)
		}

		// Buscar el archivo en el directorio
		fileInodeNum, err := findFileInDirectoryEXT2(file, ext2fs, parentInode, fileName)
		if err != nil {
			return fmt.Errorf("error al buscar el archivo: %v", err)
		}
		if fileInodeNum == -1 {
			return fmt.Errorf("el archivo '%s' no existe", fileName)
		}

		// Leer el inodo del archivo
		fileInode := &estructuraArchivoCarpeta.Inodo{}
		err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+fileInodeNum*ext2fs.SuperBlock.S_inode_s), fileInode)
		if err != nil {
			return fmt.Errorf("error al leer el inodo del archivo: %v", err)
		}

		// Verificar que sea un archivo y no un directorio
		if fileInode.Itype != 1 { // 1 = archivo
			return fmt.Errorf("'%s' no es un archivo", fileName)
		}

		// Verificar permisos de lectura y escritura
		canRead, err := sessionP2.CheckPermissions(fileInode, sessionP2.READ)
		if err != nil || !canRead {
			return fmt.Errorf("el usuario no tiene permisos de lectura sobre el archivo")
		}

		canWrite, err := sessionP2.CheckPermissions(fileInode, sessionP2.WRITE)
		if err != nil || !canWrite {
			return fmt.Errorf("el usuario no tiene permisos de escritura sobre el archivo")
		}

		// Actualizar el archivo con el nuevo contenido
		err = updateFileContentEXT2(file, ext2fs, fileInode, fileInodeNum, newContent)
		if err != nil {
			return fmt.Errorf("error al actualizar el contenido del archivo: %v", err)
		}

		// Actualizar el superbloque
		err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_inode_start+fileInodeNum*ext2fs.SuperBlock.S_inode_s), fileInode)
		if err != nil {
			return fmt.Errorf("error al escribir el inodo actualizado: %v", err)
		}

		return nil

	case 3:
		// Leer el sistema EXT3
		superbloque := &estructuraArchivoCarpeta.SuperBloque{}
		err = utilitiesP2.ReadStruct(file, int64(partition.Start), superbloque)
		if err != nil {
			return fmt.Errorf("error al leer el superbloque EXT3: %v", err)
		}

		// Calcular el tamaño de la partición basado en el superbloque
		partitionSize := EstructuraExt3P2.CalcularPartitionSizeEXT3(superbloque.S_inodes_count, superbloque.S_blocks_count)

		ext3fs := &EstructuraExt3P2.EXT3FileSystem{
			PartitionPath: partition.Path,
			PartitionSize: partitionSize,
			StartOffset:   partition.Start,
			SuperBlock:    superbloque,
		}

		// Buscar el directorio padre
		parentInodeNum, err := findDirectoryInEXT3(file, ext3fs, parentPath)
		if err != nil {
			return fmt.Errorf("error al buscar el directorio padre: %v", err)
		}

		// Leer el inodo del directorio padre
		parentInode := &estructuraArchivoCarpeta.Inodo{}
		err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+parentInodeNum*ext3fs.SuperBlock.S_inode_s), parentInode)
		if err != nil {
			return fmt.Errorf("error al leer el inodo del directorio padre: %v", err)
		}

		// Buscar el archivo en el directorio
		fileInodeNum, err := findFileInDirectoryEXT3(file, ext3fs, parentInode, fileName)
		if err != nil {
			return fmt.Errorf("error al buscar el archivo: %v", err)
		}
		if fileInodeNum == -1 {
			return fmt.Errorf("el archivo '%s' no existe", fileName)
		}

		// Leer el inodo del archivo
		fileInode := &estructuraArchivoCarpeta.Inodo{}
		err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+fileInodeNum*ext3fs.SuperBlock.S_inode_s), fileInode)
		if err != nil {
			return fmt.Errorf("error al leer el inodo del archivo: %v", err)
		}

		// Verificar que sea un archivo y no un directorio
		if fileInode.Itype != 1 { // 1 = archivo
			return fmt.Errorf("'%s' no es un archivo", fileName)
		}

		// Verificar permisos de lectura y escritura
		canRead, err := sessionP2.CheckPermissions(fileInode, sessionP2.READ)
		if err != nil || !canRead {
			return fmt.Errorf("el usuario no tiene permisos de lectura sobre el archivo")
		}

		canWrite, err := sessionP2.CheckPermissions(fileInode, sessionP2.WRITE)
		if err != nil || !canWrite {
			return fmt.Errorf("el usuario no tiene permisos de escritura sobre el archivo")
		}

		// Actualizar el archivo con el nuevo contenido
		err = updateFileContentEXT3(file, ext3fs, fileInode, fileInodeNum, newContent)
		if err != nil {
			return fmt.Errorf("error al actualizar el contenido del archivo: %v", err)
		}

		// Registrar la operación en el journal
		LogEXT3Operation(EstructuraExt3P2.OPERATION_EDIT, filePath, newContent)

		// Actualizar el superbloque
		err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_inode_start+fileInodeNum*ext3fs.SuperBlock.S_inode_s), fileInode)
		if err != nil {
			return fmt.Errorf("error al escribir el inodo actualizado: %v", err)
		}

		return nil
	}

	return fmt.Errorf("sistema de archivos no soportado")
}

// findDirectoryInEXT2 busca un directorio en el sistema EXT2
func findDirectoryInEXT2(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, dirPath string) (int32, error) {
	// Si es la raíz
	if dirPath == "" || dirPath == "/" {
		return 0, nil // El inodo 0 es la raíz en EXT2
	}

	// Dividir el path en componentes
	components := strings.Split(strings.Trim(dirPath, "/"), "/")
	currentInodeNum := int32(0) // Comenzar desde la raíz

	for _, component := range components {
		// Leer el inodo actual
		currentInode := &estructuraArchivoCarpeta.Inodo{}
		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+currentInodeNum*ext2fs.SuperBlock.S_inode_s), currentInode)
		if err != nil {
			return -1, fmt.Errorf("error al leer inodo: %v", err)
		}

		// Verificar que sea un directorio
		if currentInode.Itype != 0 { // 0 = directorio
			return -1, fmt.Errorf("'%s' no es un directorio", component)
		}

		// Buscar el componente en el directorio actual
		found := false
		nextInodeNum := int32(-1)

		// Recorrer los bloques directos del inodo
		for i := 0; i < 12 && !found; i++ {
			if currentInode.Iblock[i] == -1 {
				continue
			}

			// Leer el bloque de carpeta
			dirBlock := &bloques.BloqueCarpeta{}
			err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+currentInode.Iblock[i]*ext2fs.SuperBlock.S_block_s), dirBlock)
			if err != nil {
				return -1, fmt.Errorf("error al leer bloque de carpeta: %v", err)
			}

			// Buscar la entrada en el bloque de carpeta
			for j := 0; j < 4; j++ {
				if dirBlock.B_content[j].B_inodo == -1 {
					continue
				}

				entryName := string(dirBlock.B_content[j].B_name[:])
				entryName = strings.TrimRight(entryName, "\x00")

				if entryName == component {
					found = true
					nextInodeNum = dirBlock.B_content[j].B_inodo
					break
				}
			}
		}

		// Si no se encontró, revisar apuntadores indirectos
		if !found {
			// Revisar apuntador indirecto simple (índice 12)
			if currentInode.Iblock[12] != -1 {
				// Leer el bloque de apuntadores indirectos
				indirectBlock := &bloques.BloqueApuntador{}
				err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+currentInode.Iblock[12]*ext2fs.SuperBlock.S_block_s), indirectBlock)
				if err != nil {
					return -1, fmt.Errorf("error al leer bloque de apuntadores: %v", err)
				}

				// Revisar cada apuntador en el bloque
				for _, pointer := range indirectBlock.B_pointers {
					if pointer == -1 {
						continue
					}

					// Leer el bloque de carpeta apuntado
					dirBlock := &bloques.BloqueCarpeta{}
					err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+pointer*ext2fs.SuperBlock.S_block_s), dirBlock)
					if err != nil {
						continue
					}

					// Buscar la entrada en el bloque de carpeta
					for j := 0; j < 4; j++ {
						if dirBlock.B_content[j].B_inodo == -1 {
							continue
						}

						entryName := string(dirBlock.B_content[j].B_name[:])
						entryName = strings.TrimRight(entryName, "\x00")

						if entryName == component {
							found = true
							nextInodeNum = dirBlock.B_content[j].B_inodo
							break
						}
					}

					if found {
						break
					}
				}
			}

			if !found {
				return -1, fmt.Errorf("directorio '%s' no encontrado", component)
			}
		}

		currentInodeNum = nextInodeNum
	}

	return currentInodeNum, nil
}

// findDirectoryInEXT3 busca un directorio en el sistema EXT3
func findDirectoryInEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirPath string) (int32, error) {
	// Si es la raíz
	if dirPath == "" || dirPath == "/" {
		return 0, nil // El inodo 0 es la raíz en EXT3
	}

	// Dividir el path en componentes
	components := strings.Split(strings.Trim(dirPath, "/"), "/")
	currentInodeNum := int32(0) // Comenzar desde la raíz

	for _, component := range components {
		// Leer el inodo actual
		currentInode := &estructuraArchivoCarpeta.Inodo{}
		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+currentInodeNum*ext3fs.SuperBlock.S_inode_s), currentInode)
		if err != nil {
			return -1, fmt.Errorf("error al leer inodo: %v", err)
		}

		// Verificar que sea un directorio
		if currentInode.Itype != 0 { // 0 = directorio
			return -1, fmt.Errorf("'%s' no es un directorio", component)
		}

		// Buscar el componente en el directorio actual
		found := false
		nextInodeNum := int32(-1)

		// Recorrer los bloques directos del inodo
		for i := 0; i < 12 && !found; i++ {
			if currentInode.Iblock[i] == -1 {
				continue
			}

			// Leer el bloque de carpeta
			dirBlock := &bloques.BloqueCarpeta{}
			err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+currentInode.Iblock[i]*ext3fs.SuperBlock.S_block_s), dirBlock)
			if err != nil {
				return -1, fmt.Errorf("error al leer bloque de carpeta: %v", err)
			}

			// Buscar la entrada en el bloque de carpeta
			for j := 0; j < 4; j++ {
				if dirBlock.B_content[j].B_inodo == -1 {
					continue
				}

				entryName := string(dirBlock.B_content[j].B_name[:])
				entryName = strings.TrimRight(entryName, "\x00")

				if entryName == component {
					found = true
					nextInodeNum = dirBlock.B_content[j].B_inodo
					break
				}
			}
		}

		// Si no se encontró, revisar apuntadores indirectos
		if !found {
			// Revisar apuntador indirecto simple (índice 12)
			if currentInode.Iblock[12] != -1 {
				// Leer el bloque de apuntadores indirectos
				indirectBlock := &bloques.BloqueApuntador{}
				err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+currentInode.Iblock[12]*ext3fs.SuperBlock.S_block_s), indirectBlock)
				if err != nil {
					return -1, fmt.Errorf("error al leer bloque de apuntadores: %v", err)
				}

				// Revisar cada apuntador en el bloque
				for _, pointer := range indirectBlock.B_pointers {
					if pointer == -1 {
						continue
					}

					// Leer el bloque de carpeta apuntado
					dirBlock := &bloques.BloqueCarpeta{}
					err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+pointer*ext3fs.SuperBlock.S_block_s), dirBlock)
					if err != nil {
						continue
					}

					// Buscar la entrada en el bloque de carpeta
					for j := 0; j < 4; j++ {
						if dirBlock.B_content[j].B_inodo == -1 {
							continue
						}

						entryName := string(dirBlock.B_content[j].B_name[:])
						entryName = strings.TrimRight(entryName, "\x00")

						if entryName == component {
							found = true
							nextInodeNum = dirBlock.B_content[j].B_inodo
							break
						}
					}

					if found {
						break
					}
				}
			}

			if !found {
				return -1, fmt.Errorf("directorio '%s' no encontrado", component)
			}
		}

		currentInodeNum = nextInodeNum
	}

	return currentInodeNum, nil
}

// findFileInDirectoryEXT2 busca un archivo en un directorio específico en EXT2
func findFileInDirectoryEXT2(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, dirInode *estructuraArchivoCarpeta.Inodo, fileName string) (int32, error) {
	// Verificar que el inodo sea un directorio
	if dirInode.Itype != 0 { // 0 = directorio
		return -1, fmt.Errorf("el inodo no es un directorio")
	}

	// Buscar el archivo en los bloques directos
	for i := 0; i < 12; i++ {
		if dirInode.Iblock[i] == -1 {
			continue
		}

		// Leer el bloque de carpeta
		dirBlock := &bloques.BloqueCarpeta{}
		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+dirInode.Iblock[i]*ext2fs.SuperBlock.S_block_s), dirBlock)
		if err != nil {
			return -1, fmt.Errorf("error al leer bloque de carpeta: %v", err)
		}

		// Buscar la entrada en el bloque de carpeta
		for j := 0; j < 4; j++ {
			if dirBlock.B_content[j].B_inodo == -1 {
				continue
			}

			entryName := string(dirBlock.B_content[j].B_name[:])
			entryName = strings.TrimRight(entryName, "\x00")

			if entryName == fileName {
				return dirBlock.B_content[j].B_inodo, nil
			}
		}
	}

	// Si no se encontró, revisar apuntadores indirectos
	// Revisar apuntador indirecto simple (índice 12)
	if dirInode.Iblock[12] != -1 {
		// Leer el bloque de apuntadores indirectos
		indirectBlock := &bloques.BloqueApuntador{}
		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+dirInode.Iblock[12]*ext2fs.SuperBlock.S_block_s), indirectBlock)
		if err != nil {
			return -1, fmt.Errorf("error al leer bloque de apuntadores: %v", err)
		}

		// Revisar cada apuntador en el bloque
		for _, pointer := range indirectBlock.B_pointers {
			if pointer == -1 {
				continue
			}

			// Leer el bloque de carpeta apuntado
			dirBlock := &bloques.BloqueCarpeta{}
			err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+pointer*ext2fs.SuperBlock.S_block_s), dirBlock)
			if err != nil {
				continue
			}

			// Buscar la entrada en el bloque de carpeta
			for j := 0; j < 4; j++ {
				if dirBlock.B_content[j].B_inodo == -1 {
					continue
				}

				entryName := string(dirBlock.B_content[j].B_name[:])
				entryName = strings.TrimRight(entryName, "\x00")

				if entryName == fileName {
					return dirBlock.B_content[j].B_inodo, nil
				}
			}
		}
	}

	return -1, nil // No se encontró el archivo
}

// findFileInDirectoryEXT3 busca un archivo en un directorio específico en EXT3
func findFileInDirectoryEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInode *estructuraArchivoCarpeta.Inodo, fileName string) (int32, error) {
	// Verificar que el inodo sea un directorio
	if dirInode.Itype != 0 { // 0 = directorio
		return -1, fmt.Errorf("el inodo no es un directorio")
	}

	// Buscar el archivo en los bloques directos
	for i := 0; i < 12; i++ {
		if dirInode.Iblock[i] == -1 {
			continue
		}

		// Leer el bloque de carpeta
		dirBlock := &bloques.BloqueCarpeta{}
		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+dirInode.Iblock[i]*ext3fs.SuperBlock.S_block_s), dirBlock)
		if err != nil {
			return -1, fmt.Errorf("error al leer bloque de carpeta: %v", err)
		}

		// Buscar la entrada en el bloque de carpeta
		for j := 0; j < 4; j++ {
			if dirBlock.B_content[j].B_inodo == -1 {
				continue
			}

			entryName := string(dirBlock.B_content[j].B_name[:])
			entryName = strings.TrimRight(entryName, "\x00")

			if entryName == fileName {
				return dirBlock.B_content[j].B_inodo, nil
			}
		}
	}

	// Si no se encontró, revisar apuntadores indirectos
	// Revisar apuntador indirecto simple (índice 12)
	if dirInode.Iblock[12] != -1 {
		// Leer el bloque de apuntadores indirectos
		indirectBlock := &bloques.BloqueApuntador{}
		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+dirInode.Iblock[12]*ext3fs.SuperBlock.S_block_s), indirectBlock)
		if err != nil {
			return -1, fmt.Errorf("error al leer bloque de apuntadores: %v", err)
		}

		// Revisar cada apuntador en el bloque
		for _, pointer := range indirectBlock.B_pointers {
			if pointer == -1 {
				continue
			}

			// Leer el bloque de carpeta apuntado
			dirBlock := &bloques.BloqueCarpeta{}
			err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+pointer*ext3fs.SuperBlock.S_block_s), dirBlock)
			if err != nil {
				continue
			}

			// Buscar la entrada en el bloque de carpeta
			for j := 0; j < 4; j++ {
				if dirBlock.B_content[j].B_inodo == -1 {
					continue
				}

				entryName := string(dirBlock.B_content[j].B_name[:])
				entryName = strings.TrimRight(entryName, "\x00")

				if entryName == fileName {
					return dirBlock.B_content[j].B_inodo, nil
				}
			}
		}
	}

	return -1, nil // No se encontró el archivo
}

// updateFileContentEXT2 actualiza el contenido de un archivo en EXT2
func updateFileContentEXT2(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, fileInode *estructuraArchivoCarpeta.Inodo, fileInodeNum int32, content string) error {
	// Liberar todos los bloques actuales del archivo
	for i := 0; i < 15; i++ {
		if fileInode.Iblock[i] != -1 {
			// Marcar el bloque como libre en el bitmap
			blockBitmap, err := utilitiesP2.ReadBytes(file, int64(ext2fs.SuperBlock.S_bm_block_start+fileInode.Iblock[i]/8), 1)
			if err != nil {
				return fmt.Errorf("error al leer bitmap de bloques: %v", err)
			}

			// Liberar el bloque en el bitmap
			blockBitmap[0] &= ^(1 << uint(fileInode.Iblock[i]%8))

			// Escribir el bitmap actualizado
			err = utilitiesP2.WriteBytes(file, int64(ext2fs.SuperBlock.S_bm_block_start+fileInode.Iblock[i]/8), blockBitmap)
			if err != nil {
				return fmt.Errorf("error al escribir bitmap de bloques: %v", err)
			}

			// Actualizar contador de bloques libres
			ext2fs.SuperBlock.S_free_blocks_count++

			// Marcar el bloque como libre
			fileInode.Iblock[i] = -1
		}
	}

	// Reset del tamaño del archivo
	fileInode.Is = 0

	// Dividir el contenido en bloques de 64 bytes
	contentBytes := []byte(content)
	contentSize := int32(len(contentBytes))
	fileInode.Is = contentSize

	blockCount := (contentSize + 63) / 64 // Redondeo hacia arriba
	currentPos := 0

	// Asignar nuevos bloques directos
	for i := 0; i < 12 && i < int(blockCount); i++ {
		// Buscar un bloque libre
		newBlockNum, err := EstructuraExt2.FindFreeBlockEXT2(file, ext2fs)
		if err != nil {
			return fmt.Errorf("no hay bloques libres disponibles: %v", err)
		}

		// Marcar el bloque como usado en el bitmap
		err = markBlockAsUsedEXT2(file, ext2fs, newBlockNum)
		if err != nil {
			return fmt.Errorf("error al marcar bloque como usado: %v", err)
		}

		// Asignar el bloque al inodo
		fileInode.Iblock[i] = newBlockNum

		// Calcular cuánto contenido escribir en este bloque
		bytesToWrite := 64
		if currentPos+bytesToWrite > int(contentSize) {
			bytesToWrite = int(contentSize) - currentPos
		}

		// Crear un bloque de archivo con el contenido
		blockContent := contentBytes[currentPos : currentPos+bytesToWrite]
		fileBlock := &bloques.BloqueArchivo{}

		// Copiar el contenido al bloque
		for j := 0; j < bytesToWrite; j++ {
			fileBlock.B_content[j] = blockContent[j]
		}

		// Escribir el bloque en el disco
		err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+newBlockNum*ext2fs.SuperBlock.S_block_s), fileBlock)
		if err != nil {
			return fmt.Errorf("error al escribir bloque de archivo: %v", err)
		}

		currentPos += bytesToWrite
	}

	// Actualizar el superbloque
	ext2fs.SuperBlock.S_free_blocks_count -= blockCount
	err := utilitiesP2.WriteStruct(file, int64(ext2fs.StartOffset), ext2fs.SuperBlock)
	if err != nil {
		return fmt.Errorf("error al actualizar superbloque: %v", err)
	}

	// Actualizar el inodo del archivo
	return utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_inode_start+fileInodeNum*ext2fs.SuperBlock.S_inode_s), fileInode)
}

// updateFileContentEXT3 actualiza el contenido de un archivo en EXT3
func updateFileContentEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, fileInode *estructuraArchivoCarpeta.Inodo, fileInodeNum int32, content string) error {
	// Liberar todos los bloques actuales del archivo
	for i := 0; i < 15; i++ {
		if fileInode.Iblock[i] != -1 {
			// Marcar el bloque como libre en el bitmap
			blockBitmap, err := utilitiesP2.ReadBytes(file, int64(ext3fs.SuperBlock.S_bm_block_start+fileInode.Iblock[i]/8), 1)
			if err != nil {
				return fmt.Errorf("error al leer bitmap de bloques: %v", err)
			}

			// Liberar el bloque en el bitmap
			blockBitmap[0] &= ^(1 << uint(fileInode.Iblock[i]%8))

			// Escribir el bitmap actualizado
			err = utilitiesP2.WriteBytes(file, int64(ext3fs.SuperBlock.S_bm_block_start+fileInode.Iblock[i]/8), blockBitmap)
			if err != nil {
				return fmt.Errorf("error al escribir bitmap de bloques: %v", err)
			}

			// Actualizar contador de bloques libres
			ext3fs.SuperBlock.S_free_blocks_count++

			// Marcar el bloque como libre
			fileInode.Iblock[i] = -1
		}
	}

	// Reset del tamaño del archivo
	fileInode.Is = 0

	// Dividir el contenido en bloques de 64 bytes
	contentBytes := []byte(content)
	contentSize := int32(len(contentBytes))
	fileInode.Is = contentSize

	blockCount := (contentSize + 63) / 64 // Redondeo hacia arriba
	currentPos := 0

	// Asignar nuevos bloques directos
	for i := 0; i < 12 && i < int(blockCount); i++ {
		// Buscar un bloque libre
		newBlockNum, err := findFreeBlockEXT3(file, ext3fs)
		if err != nil {
			return fmt.Errorf("no hay bloques libres disponibles: %v", err)
		}

		// Marcar el bloque como usado en el bitmap
		err = markBlockAsUsedEXT3(file, ext3fs, newBlockNum)
		if err != nil {
			return fmt.Errorf("error al marcar bloque como usado: %v", err)
		}

		// Asignar el bloque al inodo
		fileInode.Iblock[i] = newBlockNum

		// Calcular cuánto contenido escribir en este bloque
		bytesToWrite := 64
		if currentPos+bytesToWrite > int(contentSize) {
			bytesToWrite = int(contentSize) - currentPos
		}

		// Crear un bloque de archivo con el contenido
		blockContent := contentBytes[currentPos : currentPos+bytesToWrite]
		fileBlock := &bloques.BloqueArchivo{}

		// Copiar el contenido al bloque
		for j := 0; j < bytesToWrite; j++ {
			fileBlock.B_content[j] = blockContent[j]
		}

		// Escribir el bloque en el disco
		err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+newBlockNum*ext3fs.SuperBlock.S_block_s), fileBlock)
		if err != nil {
			return fmt.Errorf("error al escribir bloque de archivo: %v", err)
		}

		currentPos += bytesToWrite
	}

	// Actualizar el superbloque
	ext3fs.SuperBlock.S_free_blocks_count -= blockCount
	err := utilitiesP2.WriteStruct(file, int64(ext3fs.StartOffset), ext3fs.SuperBlock)
	if err != nil {
		return fmt.Errorf("error al actualizar superbloque: %v", err)
	}

	// Actualizar el inodo del archivo
	return utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_inode_start+fileInodeNum*ext3fs.SuperBlock.S_inode_s), fileInode)
}

// findFreeBlockEXT3 encuentra un bloque libre en EXT3 - VERSIÓN MEJORADA
func findFreeBlockEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem) (int32, error) {
	bitmapSize := (ext3fs.SuperBlock.S_blocks_count + 7) / 8
	bitmap := make([]byte, bitmapSize)

	_, err := file.ReadAt(bitmap, int64(ext3fs.SuperBlock.S_bm_block_start))
	if err != nil {
		return -1, fmt.Errorf("error leyendo bitmap de bloques: %s", err)
	}

	// Buscar un bloque libre en el bitmap
	for i := int32(0); i < ext3fs.SuperBlock.S_blocks_count; i++ {
		byteIndex := i / 8
		bitIndex := i % 8

		// Si el bit está en 0, el bloque está libre
		if (bitmap[byteIndex] & (1 << (7 - bitIndex))) == 0 {
			fmt.Printf("DEBUG findFreeBlock: Bloque libre encontrado: %d\n", i)
			return i, nil
		}
	}

	return -1, fmt.Errorf("no hay bloques libres disponibles (revisados %d bloques)", ext3fs.SuperBlock.S_blocks_count)
}

// markBlockAsUsedEXT2 marca un bloque como usado en el bitmap de EXT2
func markBlockAsUsedEXT2(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, blockNum int32) error {
	bytePos := blockNum / 8
	bitPos := blockNum % 8

	// Leer el byte actual del bitmap
	bitmap, err := utilitiesP2.ReadBytes(file, int64(ext2fs.SuperBlock.S_bm_block_start+bytePos), 1)
	if err != nil {
		return err
	}

	// Marcar el bit correspondiente como usado (1)
	bitmap[0] |= (1 << uint(bitPos))

	// Escribir el byte actualizado
	return utilitiesP2.WriteBytes(file, int64(ext2fs.SuperBlock.S_bm_block_start+bytePos), bitmap)
}

// markBlockAsUsedEXT3 marca un bloque como usado en EXT3
func markBlockAsUsedEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, blockNum int32) error {
	byteIndex := blockNum / 8
	bitIndex := blockNum % 8

	// Leer el byte específico del bitmap
	_, err := file.Seek(int64(ext3fs.SuperBlock.S_bm_block_start)+int64(byteIndex), 0)
	if err != nil {
		return fmt.Errorf("error seeking bitmap: %s", err)
	}

	var currentByte [1]byte
	_, err = file.Read(currentByte[:])
	if err != nil {
		return fmt.Errorf("error reading bitmap byte: %s", err)
	}

	// Verificar si el bloque ya está marcado como usado
	mask := byte(1 << (7 - bitIndex))
	if (currentByte[0] & mask) != 0 {
		fmt.Printf("⚠️  ADVERTENCIA: Bloque %d ya estaba marcado como usado en el bitmap\n", blockNum)
	}

	// Modificar el bit (poner en 1 = usado)
	currentByte[0] |= mask

	// Escribir el byte modificado
	_, err = file.Seek(int64(ext3fs.SuperBlock.S_bm_block_start)+int64(byteIndex), 0)
	if err != nil {
		return fmt.Errorf("error seeking bitmap for write: %s", err)
	}

	_, err = file.Write(currentByte[:])
	if err != nil {
		return fmt.Errorf("error writing bitmap byte: %s", err)
	}

	fmt.Printf("DEBUG markBlockAsUsed: Bloque %d marcado como usado en bitmap\n", blockNum)
	return nil
}
