package commandsP2

import (
	bloques "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	session "MIA_2S2025_P2_202307820/Session"
	estructuraArchivoCarpeta "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"strings"
)

// REMOVE implementa el comando para eliminar archivos o carpetas
func REMOVE(args []string) {
	// 1. Verificar que hay una sesión activa
	if !session.IsSessionActive() {
		fmt.Println("Error: Debe iniciar sesión para usar este comando")
		fmt.Println("Use: login -user=<usuario> -pass=<contraseña> -id=<partition_id>")
		return
	}

	// 2. Obtener información de la sesión
	sessionInfo := session.GetCurrentSession()
	if sessionInfo == nil {
		fmt.Println("Error: No se pudo obtener la información de la sesión")
		return
	}

	// 3. Parsear parámetros
	params := utilitiesP2.ParseParameters(args)
	path := strings.Trim(params["path"], "\"")

	// 4. Validar parámetros
	if path == "" {
		fmt.Println("Error: Debe especificar la ruta del archivo o carpeta a eliminar")
		fmt.Println("Uso: remove -path=<ruta>")
		return
	}

	// 5. Validar que la ruta sea absoluta
	if !strings.HasPrefix(path, "/") {
		fmt.Println("Error: La ruta debe ser absoluta y comenzar con '/'")
		return
	}

	// 6. Obtener la partición montada
	mountedPartition, exists := GetMountedPartition(sessionInfo.PartitionID)
	if !exists {
		fmt.Printf("Error: La partición '%s' no está montada\n", sessionInfo.PartitionID)
		return
	}

	// 7. Verificar el tipo de sistema de archivos (EXT2 o EXT3)
	var err error
	fileSystemType, err := GetFileSystemType(mountedPartition)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

	// 8. Eliminar archivo o carpeta según el tipo de sistema de archivos
	if fileSystemType == estructuraArchivoCarpeta.EXT2_FILESYSTEM_TYPE {
		// Sistema de archivos EXT2
		err = RemoveInEXT2(mountedPartition, path, sessionInfo)
	} else if fileSystemType == estructuraArchivoCarpeta.EXT3_FILESYSTEM_TYPE {
		// Sistema de archivos EXT3
		err = RemoveInEXT3(mountedPartition, path, sessionInfo)
	} else {
		fmt.Printf("Error: Tipo de sistema de archivos desconocido o no compatible\n")
		return
	}

	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

	fmt.Printf("El elemento '%s' ha sido eliminado exitosamente\n", path)
}

// RemoveInEXT2 elimina un archivo o carpeta en el sistema de archivos EXT2
func RemoveInEXT2(partition MountedPartition, path string, sessionInfo *session.UserSession) error {
	// 1. Abrir el archivo del disco
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

	// 2. Leer el sistema EXT2 de la partición
	ext2fs, err := EstructuraExt2.ReadEXT2FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return fmt.Errorf("error leyendo sistema EXT2: %s", err)
	}

	// 3. Verificar que el sistema esté formateado
	if ext2fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return fmt.Errorf("la partición no tiene formato EXT2 válido")
	}

	// 4. Eliminar el archivo o carpeta
	return removePathInEXT2(file, ext2fs, path, sessionInfo)
}

// RemoveInEXT3 elimina un archivo o carpeta en el sistema de archivos EXT3
func RemoveInEXT3(partition MountedPartition, path string, sessionInfo *session.UserSession) error {
	// 1. Abrir el archivo del disco
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

	// 2. Leer el sistema EXT3 de la partición
	ext3fs, err := EstructuraExt3P2.ReadEXT3FileSystem(partition.Path, getPartitionStartOffset(partition))
	if err != nil {
		return fmt.Errorf("error leyendo sistema EXT3: %s", err)
	}

	// 3. Verificar que el sistema esté formateado
	if ext3fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return fmt.Errorf("la partición no tiene formato EXT3 válido")
	}

	// 4. Registrar la operación en el journal (específico de EXT3)
	// Añadir entrada al journal antes de realizar la operación
	var operation string
	if isDirectory(file, ext3fs.SuperBlock, path) {
		operation = EstructuraExt3P2.OPERATION_RMDIR
	} else {
		operation = EstructuraExt3P2.OPERATION_RMFILE
	}

	// 5. Eliminar el archivo o carpeta (reutiliza la función de EXT2)
	err = removePathInEXT2(file, &EstructuraExt2.EXT2FileSystem{
		SuperBlock:    ext3fs.SuperBlock,
		PartitionPath: ext3fs.PartitionPath,
		PartitionSize: ext3fs.PartitionSize,
		StartOffset:   ext3fs.StartOffset,
	}, path, sessionInfo)

	// Registrar el éxito en el journal
	LogEXT3Operation(operation, path, "-")
	return nil
}

// isDirectory verifica si una ruta corresponde a un directorio
func isDirectory(file *os.File, superBlock *estructuraArchivoCarpeta.SuperBloque, path string) bool {
	// Lógica simplificada: verificar el último inodo del path
	components := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// Si es raíz, es directorio
	if len(components) == 1 && components[0] == "" {
		return true
	}

	return false // Implementar lógica completa si es necesario
}

// removePathInEXT2 elimina un archivo o directorio del sistema de archivos EXT2
func removePathInEXT2(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, path string, sessionInfo *session.UserSession) error {
	// 1. Separar el path en componentes
	components := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(components) == 1 && components[0] == "" {
		return fmt.Errorf("no se puede eliminar el directorio raíz")
	}

	// 2. Obtener el directorio padre y el nombre del elemento a eliminar
	var parentPath string
	fileName := components[len(components)-1]
	if len(components) > 1 {
		parentPath = "/" + strings.Join(components[:len(components)-1], "/")
	} else {
		parentPath = "/"
	}

	// 3. Encontrar el inodo del directorio padre
	parentInodeNum, err := findDirectoryByPath(file, ext2fs, parentPath)
	if err != nil {
		return fmt.Errorf("error encontrando directorio padre '%s': %s", parentPath, err)
	}

	// 4. Leer el inodo del directorio padre
	parentInode := &estructuraArchivoCarpeta.Inodo{}
	parentInodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (parentInodeNum * ext2fs.SuperBlock.S_inode_s))
	err = utilitiesP2.ReadStruct(file, parentInodeOffset, parentInode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo del directorio padre: %s", err)
	}

	// 5. Verificar permisos de escritura en el directorio padre
	if !session.CanWrite(parentInode) {
		return fmt.Errorf("el usuario '%s' no tiene permisos para eliminar elementos en '%s'", sessionInfo.Username, parentPath)
	}

	// 6. Buscar el archivo o directorio a eliminar
	targetInodeNum, err := searchFileInDirectory(file, ext2fs, parentInodeNum, fileName)
	if err != nil {
		return fmt.Errorf("no se encontró '%s' en el directorio '%s': %s", fileName, parentPath, err)
	}

	// 7. Leer el inodo del elemento a eliminar
	targetInode := &estructuraArchivoCarpeta.Inodo{}
	targetInodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (targetInodeNum * ext2fs.SuperBlock.S_inode_s))
	err = utilitiesP2.ReadStruct(file, targetInodeOffset, targetInode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo del elemento a eliminar: %s", err)
	}

	// 8. Verificar permisos de escritura en el elemento a eliminar
	if !session.CanWrite(targetInode) {
		return fmt.Errorf("el usuario '%s' no tiene permisos para eliminar '%s'", sessionInfo.Username, path)
	}

	// 9. Determinar si es archivo o directorio y eliminar
	isDirectory := targetInode.IsDirectory()
	if isDirectory {
		// Es un directorio, eliminar recursivamente
		err = removeDirectoryRecursive(file, ext2fs, targetInodeNum, targetInode, path, sessionInfo)
		if err != nil {
			return err
		}
	} else {
		// Es un archivo, eliminar directamente
		err = removeFile(file, ext2fs, targetInodeNum, targetInode, parentInodeNum)
		if err != nil {
			return err
		}
	}

	// 10. Actualizar el superbloque
	ext2fs.SuperBlock.S_free_blocks_count++
	ext2fs.SuperBlock.S_free_inodes_count++
	superBlockOffset := int64(ext2fs.StartOffset)
	err = utilitiesP2.WriteStruct(file, superBlockOffset, ext2fs.SuperBlock)
	if err != nil {
		return fmt.Errorf("error actualizando superbloque: %s", err)
	}

	return nil
}

// removeDirectoryRecursive elimina un directorio y todo su contenido de forma recursiva
func removeDirectoryRecursive(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, dirInodeNum int32, dirInode *estructuraArchivoCarpeta.Inodo, path string, sessionInfo *session.UserSession) error {
	// 1. Obtener el contenido del directorio
	entries, err := listDirectoryContents(file, ext2fs, dirInodeNum, dirInode)
	if err != nil {
		return fmt.Errorf("error listando contenido del directorio '%s': %s", path, err)
	}

	// 2. Para cada entrada del directorio (excepto . y ..)
	for _, entry := range entries {
		entryName := entry.Name
		entryInodeNum := entry.InodeNum

		// Saltar las entradas . y ..
		if entryName == "." || entryName == ".." {
			continue
		}

		// Construir la ruta completa
		entryPath := path + "/" + entryName
		if path == "/" {
			entryPath = "/" + entryName
		}

		// Leer el inodo de la entrada
		entryInode := &estructuraArchivoCarpeta.Inodo{}
		entryInodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (entryInodeNum * ext2fs.SuperBlock.S_inode_s))
		err = utilitiesP2.ReadStruct(file, entryInodeOffset, entryInode)
		if err != nil {
			return fmt.Errorf("error leyendo inodo de '%s': %s", entryPath, err)
		}

		// Verificar permisos
		if !session.CanWrite(entryInode) {
			fmt.Printf("Aviso: No se puede eliminar '%s': el usuario no tiene permisos de escritura\n", entryPath)
			return fmt.Errorf("no se puede eliminar el directorio '%s' porque contiene elementos sin permiso de escritura", path)
		}

		// Eliminar recursivamente si es directorio, directamente si es archivo
		if entryInode.IsDirectory() { // Es directorio
			err = removeDirectoryRecursive(file, ext2fs, entryInodeNum, entryInode, entryPath, sessionInfo)
			if err != nil {
				return err
			}
		} else { // Es archivo
			err = removeFile(file, ext2fs, entryInodeNum, entryInode, dirInodeNum)
			if err != nil {
				return err
			}
		}
	}

	// 3. Finalmente, eliminar el directorio vacío
	parentPath := path[:strings.LastIndex(path, "/")]
	if parentPath == "" {
		parentPath = "/"
	}

	parentInodeNum, err := findDirectoryByPath(file, ext2fs, parentPath)
	if err != nil {
		return fmt.Errorf("error encontrando directorio padre '%s': %s", parentPath, err)
	}

	return removeEmptyDirectory(file, ext2fs, dirInodeNum, dirInode, parentInodeNum)
}

// removeFile elimina un archivo del sistema de archivos
func removeFile(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, fileInodeNum int32, fileInode *estructuraArchivoCarpeta.Inodo, parentInodeNum int32) error {
	// 1. Liberar los bloques usados por el archivo
	blocks := fileInode.Iblock[:]
	for i := 0; i < 15; i++ {
		if blocks[i] != -1 {
			// Para bloques indirectos, liberar también los bloques referenciados
			if i >= 12 {
				err := freeIndirectBlocks(file, ext2fs, blocks[i], i-11)
				if err != nil {
					return err
				}
			}

			// Marcar el bloque como libre en el bitmap
			err := updateBlockBitmaps(file, ext2fs, blocks[i], false)
			if err != nil {
				return err
			}
		}
	}

	// 2. Marcar el inodo como libre
	err := updateInodeInBitmap(file, ext2fs, fileInodeNum, false)
	if err != nil {
		return err
	}

	// 3. Obtener el nombre del archivo desde el directorio padre
	fileName := getFileNameFromParent(file, ext2fs, parentInodeNum, fileInodeNum)
	if fileName == "" {
		return fmt.Errorf("no se pudo encontrar el nombre del archivo en el directorio padre")
	}

	// 4. Eliminar la entrada del directorio padre
	err = removeEntryFromDirectory(file, ext2fs, parentInodeNum, fileName, fileInodeNum)
	if err != nil {
		return err
	}

	return nil
}

// removeEmptyDirectory elimina un directorio vacío
func removeEmptyDirectory(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, dirInodeNum int32, dirInode *estructuraArchivoCarpeta.Inodo, parentInodeNum int32) error {
	// 1. Verificar que el directorio esté vacío (solo contiene . y ..)
	entries, err := listDirectoryContents(file, ext2fs, dirInodeNum, dirInode)
	if err != nil {
		return fmt.Errorf("error listando contenido del directorio: %s", err)
	}

	// Solo debería tener las entradas "." y ".."
	if len(entries) > 2 {
		nonSpecialEntries := 0
		for _, entry := range entries {
			if entry.Name != "." && entry.Name != ".." {
				nonSpecialEntries++
			}
		}

		if nonSpecialEntries > 0 {
			return fmt.Errorf("el directorio no está vacío")
		}
	}

	// 2. Liberar los bloques usados por el directorio
	blocks := dirInode.Iblock[:]
	for i := 0; i < 15; i++ {
		if blocks[i] != -1 {
			// Para bloques indirectos, liberar también los bloques referenciados
			if i >= 12 {
				err := freeIndirectBlocks(file, ext2fs, blocks[i], i-11)
				if err != nil {
					return err
				}
			}

			// Marcar el bloque como libre en el bitmap
			err := updateBlockBitmaps(file, ext2fs, blocks[i], false)
			if err != nil {
				return err
			}
		}
	}

	// 3. Marcar el inodo como libre
	err = updateInodeInBitmap(file, ext2fs, dirInodeNum, false)
	if err != nil {
		return err
	}

	// 4. Obtener el nombre del directorio desde el directorio padre
	dirName := getFileNameFromParent(file, ext2fs, parentInodeNum, dirInodeNum)
	if dirName == "" {
		return fmt.Errorf("no se pudo encontrar el nombre del directorio en el directorio padre")
	}

	// 5. Eliminar la entrada del directorio padre
	err = removeEntryFromDirectory(file, ext2fs, parentInodeNum, dirName, dirInodeNum)
	if err != nil {
		return err
	}

	return nil
}

// DirectoryEntry representa una entrada de directorio
type DirectoryEntry struct {
	Name     string
	InodeNum int32
}

// listDirectoryContents lista todas las entradas de un directorio
func listDirectoryContents(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, dirInodeNum int32, dirInode *estructuraArchivoCarpeta.Inodo) ([]DirectoryEntry, error) {
	entries := []DirectoryEntry{}

	// Recorrer todos los bloques del directorio
	blocks := dirInode.Iblock[:]

	for i := 0; i < 15; i++ {
		if blocks[i] == -1 {
			continue
		}

		// Para bloques directos
		if i < 12 {
			// Leer el bloque de directorio
			dirBlock := &bloques.BloqueCarpeta{}
			blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blocks[i] * ext2fs.SuperBlock.S_block_s))
			err := utilitiesP2.ReadStruct(file, blockOffset, dirBlock)
			if err != nil {
				return nil, fmt.Errorf("error leyendo bloque de directorio: %s", err)
			}

			// Agregar entradas válidas
			for _, content := range dirBlock.B_content {
				if content.B_inodo != -1 {
					name := stringFromBytes(content.B_name[:])
					entries = append(entries, DirectoryEntry{
						Name:     name,
						InodeNum: content.B_inodo,
					})
				}
			}
		} else if i == 12 {
			// Indirección simple
			err := processIndirectBlock(file, ext2fs, blocks[i], 1, &entries)
			if err != nil {
				return nil, err
			}
		} else if i == 13 {
			// Indirección doble
			err := processIndirectBlock(file, ext2fs, blocks[i], 2, &entries)
			if err != nil {
				return nil, err
			}
		} else if i == 14 {
			// Indirección triple
			err := processIndirectBlock(file, ext2fs, blocks[i], 3, &entries)
			if err != nil {
				return nil, err
			}
		}
	}

	return entries, nil
}

// processIndirectBlock procesa un bloque indirecto para listar directorios
func processIndirectBlock(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, blockNum int32, level int, entries *[]DirectoryEntry) error {
	if blockNum == -1 {
		return nil
	}

	// Leer bloque de apuntadores
	apBlock := &bloques.BloqueApuntador{}
	blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
	err := utilitiesP2.ReadStruct(file, blockOffset, apBlock)
	if err != nil {
		return fmt.Errorf("error leyendo bloque de apuntadores: %s", err)
	}

	// Procesar cada apuntador
	for i := 0; i < 16; i++ {
		if apBlock.B_pointers[i] == -1 {
			continue
		}

		if level == 1 {
			// Leer el bloque de directorio
			dirBlock := &bloques.BloqueCarpeta{}
			blockOffset := int64(ext2fs.SuperBlock.S_block_start + (apBlock.B_pointers[i] * ext2fs.SuperBlock.S_block_s))
			err := utilitiesP2.ReadStruct(file, blockOffset, dirBlock)
			if err != nil {
				continue
			}

			// Agregar entradas válidas
			for _, content := range dirBlock.B_content {
				if content.B_inodo != -1 {
					name := stringFromBytes(content.B_name[:])
					*entries = append(*entries, DirectoryEntry{
						Name:     name,
						InodeNum: content.B_inodo,
					})
				}
			}
		} else {
			// Llamar recursivamente para nivel - 1
			err := processIndirectBlock(file, ext2fs, apBlock.B_pointers[i], level-1, entries)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// freeIndirectBlocks libera bloques indirectos
func freeIndirectBlocks(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, blockNum int32, level int) error {
	if level <= 0 || blockNum == -1 {
		return nil
	}

	// Leer bloque de apuntadores
	apBlock := &bloques.BloqueApuntador{}
	blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
	err := utilitiesP2.ReadStruct(file, blockOffset, apBlock)
	if err != nil {
		return fmt.Errorf("error leyendo bloque de apuntadores: %s", err)
	}

	// Para cada apuntador válido, liberar recursivamente
	for i := 0; i < 16; i++ {
		if apBlock.B_pointers[i] != -1 {
			// Si es nivel > 1, llamar recursivamente
			if level > 1 {
				err := freeIndirectBlocks(file, ext2fs, apBlock.B_pointers[i], level-1)
				if err != nil {
					return err
				}
			}

			// Marcar el bloque como libre
			err := updateBlockBitmaps(file, ext2fs, apBlock.B_pointers[i], false)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// updateBlockBitmap actualiza el bitmap de bloques
func updateBlockBitmaps(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, blockNum int32, used bool) error {
	if blockNum < 0 {
		return nil
	}

	// Calcular la posición en el bitmap
	bytePosition := blockNum / 8
	bitPosition := blockNum % 8

	// Leer el byte específico del bitmap
	bitmapOffset := int64(ext2fs.SuperBlock.S_bm_block_start + bytePosition)
	var byteValue [1]byte
	_, err := file.ReadAt(byteValue[:], bitmapOffset)
	if err != nil {
		return fmt.Errorf("error leyendo bitmap de bloques: %s", err)
	}

	// Modificar el bit
	if used {
		byteValue[0] |= (1 << bitPosition)
	} else {
		byteValue[0] &= ^(1 << bitPosition)
	}

	// Escribir el byte modificado
	_, err = file.WriteAt(byteValue[:], bitmapOffset)
	if err != nil {
		return fmt.Errorf("error escribiendo bitmap de bloques: %s", err)
	}

	return nil
}

// updateInodeInBitmap actualiza el bitmap de inodos
func updateInodeInBitmap(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, inodeNum int32, used bool) error {
	if inodeNum < 0 {
		return nil
	}

	// Calcular la posición en el bitmap
	bytePosition := inodeNum / 8
	bitPosition := inodeNum % 8

	// Leer el byte específico del bitmap
	bitmapOffset := int64(ext2fs.SuperBlock.S_bm_inode_start + bytePosition)
	var byteValue [1]byte
	_, err := file.ReadAt(byteValue[:], bitmapOffset)
	if err != nil {
		return fmt.Errorf("error leyendo bitmap de inodos: %s", err)
	}

	// Modificar el bit
	if used {
		byteValue[0] |= (1 << bitPosition)
	} else {
		byteValue[0] &= ^(1 << bitPosition)
	}

	// Escribir el byte modificado
	_, err = file.WriteAt(byteValue[:], bitmapOffset)
	if err != nil {
		return fmt.Errorf("error escribiendo bitmap de inodos: %s", err)
	}

	return nil
}

// stringFromBytes convierte un array de bytes a string, eliminando bytes nulos
func stringFromBytes(data []byte) string {
	// Encontrar el primer byte nulo (0)
	length := 0
	for i, b := range data {
		if b == 0 {
			length = i
			break
		}
		length = i + 1
	}
	return string(data[:length])
}

// getFileNameFromParent obtiene el nombre de un archivo/directorio desde su padre
func getFileNameFromParent(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, parentInodeNum int32, targetInodeNum int32) string {
	// Leer el inodo del directorio padre
	parentInode := &estructuraArchivoCarpeta.Inodo{}
	parentInodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (parentInodeNum * ext2fs.SuperBlock.S_inode_s))
	err := utilitiesP2.ReadStruct(file, parentInodeOffset, parentInode)
	if err != nil {
		return ""
	}

	// Recorrer todos los bloques del directorio
	blocks := parentInode.Iblock[:]

	// Primero buscar en bloques directos
	for i := 0; i < 12; i++ {
		if blocks[i] == -1 {
			continue
		}

		// Leer el bloque de directorio
		dirBlock := &bloques.BloqueCarpeta{}
		blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blocks[i] * ext2fs.SuperBlock.S_block_s))
		err := utilitiesP2.ReadStruct(file, blockOffset, dirBlock)
		if err != nil {
			continue
		}

		// Buscar la entrada que coincida con el inodo
		for _, content := range dirBlock.B_content {
			if content.B_inodo == targetInodeNum {
				return stringFromBytes(content.B_name[:])
			}
		}
	}

	// Buscar en bloque indirecto simple (índice 12)
	if blocks[12] != -1 {
		name := searchInIndirectBlock(file, ext2fs, blocks[12], 1, targetInodeNum)
		if name != "" {
			return name
		}
	}

	// Buscar en bloque indirecto doble (índice 13)
	if blocks[13] != -1 {
		name := searchInIndirectBlock(file, ext2fs, blocks[13], 2, targetInodeNum)
		if name != "" {
			return name
		}
	}

	// Buscar en bloque indirecto triple (índice 14)
	if blocks[14] != -1 {
		name := searchInIndirectBlock(file, ext2fs, blocks[14], 3, targetInodeNum)
		if name != "" {
			return name
		}
	}

	return ""
}

// searchInIndirectBlock busca un nombre en bloques indirectos
func searchInIndirectBlock(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, blockNum int32, level int, targetInodeNum int32) string {
	if blockNum == -1 {
		return ""
	}

	// Leer bloque de apuntadores
	apBlock := &bloques.BloqueApuntador{}
	blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
	err := utilitiesP2.ReadStruct(file, blockOffset, apBlock)
	if err != nil {
		return ""
	}

	// Procesar cada apuntador
	for i := 0; i < 16; i++ {
		if apBlock.B_pointers[i] == -1 {
			continue
		}

		if level == 1 {
			// Leer el bloque de directorio
			dirBlock := &bloques.BloqueCarpeta{}
			blockOffset := int64(ext2fs.SuperBlock.S_block_start + (apBlock.B_pointers[i] * ext2fs.SuperBlock.S_block_s))
			err := utilitiesP2.ReadStruct(file, blockOffset, dirBlock)
			if err != nil {
				continue
			}

			// Buscar la entrada que coincida con el inodo
			for _, content := range dirBlock.B_content {
				if content.B_inodo == targetInodeNum {
					return stringFromBytes(content.B_name[:])
				}
			}
		} else {
			// Llamar recursivamente para nivel - 1
			name := searchInIndirectBlock(file, ext2fs, apBlock.B_pointers[i], level-1, targetInodeNum)
			if name != "" {
				return name
			}
		}
	}

	return ""
}

// removeEntryFromDirectory elimina una entrada de directorio
func removeEntryFromDirectory(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, dirInodeNum int32, entryName string, entryInodeNum int32) error {
	// Leer el inodo del directorio
	dirInode := &estructuraArchivoCarpeta.Inodo{}
	dirInodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (dirInodeNum * ext2fs.SuperBlock.S_inode_s))
	err := utilitiesP2.ReadStruct(file, dirInodeOffset, dirInode)
	if err != nil {
		return fmt.Errorf("error leyendo inodo del directorio: %s", err)
	}

	// Recorrer todos los bloques directos del directorio
	blocks := dirInode.Iblock[:]

	for i := 0; i < 12; i++ {
		if blocks[i] == -1 {
			continue
		}

		// Leer el bloque de directorio
		dirBlock := &bloques.BloqueCarpeta{}
		blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blocks[i] * ext2fs.SuperBlock.S_block_s))
		err := utilitiesP2.ReadStruct(file, blockOffset, dirBlock)
		if err != nil {
			continue
		}

		// Buscar la entrada y marcarla como libre
		found := false
		for j := range dirBlock.B_content {
			name := stringFromBytes(dirBlock.B_content[j].B_name[:])
			if dirBlock.B_content[j].B_inodo == entryInodeNum && name == entryName {
				// Marcar la entrada como libre
				dirBlock.B_content[j].B_inodo = -1
				for k := range dirBlock.B_content[j].B_name {
					dirBlock.B_content[j].B_name[k] = 0
				}
				found = true
				break
			}
		}

		if found {
			// Escribir el bloque actualizado
			err = utilitiesP2.WriteStruct(file, blockOffset, dirBlock)
			if err != nil {
				return fmt.Errorf("error escribiendo bloque de directorio: %s", err)
			}
			return nil
		}
	}

	// Si no se encontró en bloques directos, buscar en indirectos
	if blocks[12] != -1 {
		found, err := removeEntryFromIndirectBlock(file, ext2fs, blocks[12], 1, entryName, entryInodeNum)
		if err != nil {
			return err
		}
		if found {
			return nil
		}
	}

	if blocks[13] != -1 {
		found, err := removeEntryFromIndirectBlock(file, ext2fs, blocks[13], 2, entryName, entryInodeNum)
		if err != nil {
			return err
		}
		if found {
			return nil
		}
	}

	if blocks[14] != -1 {
		found, err := removeEntryFromIndirectBlock(file, ext2fs, blocks[14], 3, entryName, entryInodeNum)
		if err != nil {
			return err
		}
		if found {
			return nil
		}
	}

	return fmt.Errorf("no se encontró la entrada '%s' en el directorio", entryName)
}

// removeEntryFromIndirectBlock elimina una entrada desde un bloque indirecto
func removeEntryFromIndirectBlock(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, blockNum int32, level int, entryName string, entryInodeNum int32) (bool, error) {
	if blockNum == -1 {
		return false, nil
	}

	// Leer bloque de apuntadores
	apBlock := &bloques.BloqueApuntador{}
	blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
	err := utilitiesP2.ReadStruct(file, blockOffset, apBlock)
	if err != nil {
		return false, fmt.Errorf("error leyendo bloque de apuntadores: %s", err)
	}

	for i := 0; i < 16; i++ {
		if apBlock.B_pointers[i] == -1 {
			continue
		}

		if level == 1 {
			// Leer el bloque de directorio
			dirBlock := &bloques.BloqueCarpeta{}
			blockOffset := int64(ext2fs.SuperBlock.S_block_start + (apBlock.B_pointers[i] * ext2fs.SuperBlock.S_block_s))
			err := utilitiesP2.ReadStruct(file, blockOffset, dirBlock)
			if err != nil {
				continue
			}

			// Buscar la entrada y marcarla como libre
			found := false
			for j := range dirBlock.B_content {
				name := stringFromBytes(dirBlock.B_content[j].B_name[:])
				if dirBlock.B_content[j].B_inodo == entryInodeNum && name == entryName {
					// Marcar la entrada como libre
					dirBlock.B_content[j].B_inodo = -1
					for k := range dirBlock.B_content[j].B_name {
						dirBlock.B_content[j].B_name[k] = 0
					}
					found = true
					break
				}
			}

			if found {
				// Escribir el bloque actualizado
				err = utilitiesP2.WriteStruct(file, blockOffset, dirBlock)
				if err != nil {
					return false, fmt.Errorf("error escribiendo bloque de directorio: %s", err)
				}
				return true, nil
			}
		} else {
			// Buscar recursivamente en niveles inferiores
			found, err := removeEntryFromIndirectBlock(file, ext2fs, apBlock.B_pointers[i], level-1, entryName, entryInodeNum)
			if err != nil {
				return false, err
			}
			if found {
				return true, nil
			}
		}
	}

	return false, nil
}

// findDirectoryByPath encuentra un directorio dado su path
func findDirectoryByPath(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, path string) (int32, error) {
	// Si es la raíz, devolver inodo 0
	if path == "/" {
		return 0, nil
	}

	// Separar el path en componentes
	components := strings.Split(strings.TrimPrefix(path, "/"), "/")

	// Comenzar desde la raíz
	currentInodeNum := int32(0)

	// Recorrer cada componente del path
	for _, component := range components {
		if component == "" {
			continue
		}

		// Buscar el componente en el directorio actual
		nextInodeNum, err := searchFileInDirectory(file, ext2fs, currentInodeNum, component)
		if err != nil {
			return -1, fmt.Errorf("error encontrando '%s': %s", component, err)
		}

		// Leer el inodo para verificar que es un directorio
		nextInode := &estructuraArchivoCarpeta.Inodo{}
		nextInodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (nextInodeNum * ext2fs.SuperBlock.S_inode_s))
		err = utilitiesP2.ReadStruct(file, nextInodeOffset, nextInode)
		if err != nil {
			return -1, fmt.Errorf("error leyendo inodo: %s", err)
		}

		// Verificar que es un directorio
		if !nextInode.IsDirectory() {
			return -1, fmt.Errorf("'%s' no es un directorio", component)
		}

		// Actualizar el inodo actual
		currentInodeNum = nextInodeNum
	}

	return currentInodeNum, nil
}

// GetFileSystemType determina si la partición tiene formato EXT2 o EXT3
func GetFileSystemType(partition MountedPartition) (int32, error) {
	file, err := os.Open(partition.Path)
	if err != nil {
		return 0, fmt.Errorf("error abriendo el disco '%s': %s", partition.Path, err)
	}
	defer file.Close()

	// Obtener el offset de inicio de la partición
	startOffset := getPartitionStartOffset(partition)

	// Leer el superbloque
	superBlock := &estructuraArchivoCarpeta.SuperBloque{}
	superBlockOffset := int64(startOffset)
	err = utilitiesP2.ReadStruct(file, superBlockOffset, superBlock)
	if err != nil {
		return 0, fmt.Errorf("error leyendo superbloque: %s", err)
	}

	// Verificar que es un sistema de archivos EXT2/EXT3 válido
	if superBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return 0, fmt.Errorf("la partición no tiene un formato válido de sistema de archivos")
	}

	return superBlock.S_filesystem_type, nil
}

// searchFileInDirectory busca un archivo o directorio en un directorio específico
func searchFileInDirectory(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, dirInodeNum int32, fileName string) (int32, error) {
	// Leer el inodo del directorio
	dirInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (dirInodeNum * ext2fs.SuperBlock.S_inode_s))
	err := utilitiesP2.ReadStruct(file, inodeOffset, dirInode)
	if err != nil {
		return -1, fmt.Errorf("error leyendo inodo del directorio: %s", err)
	}

	// Verificar que es un directorio
	if !dirInode.IsDirectory() {
		return -1, fmt.Errorf("el inodo %d no es un directorio", dirInodeNum)
	}

	// Buscar en los bloques directos
	blocks := dirInode.Iblock[:]
	for i := 0; i < 12; i++ {
		if blocks[i] != -1 {
			// Leer el bloque de directorio
			dirBlock := &bloques.BloqueCarpeta{}
			blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blocks[i] * ext2fs.SuperBlock.S_block_s))
			err := utilitiesP2.ReadStruct(file, blockOffset, dirBlock)
			if err != nil {
				continue
			}

			// Buscar la entrada en el bloque
			for _, content := range dirBlock.B_content {
				if content.B_inodo != -1 {
					name := stringFromBytes(content.B_name[:])
					if name == fileName {
						return content.B_inodo, nil
					}
				}
			}
		}
	}

	// Buscar en bloques indirectos si es necesario
	if blocks[12] != -1 {
		inodeNum, found := searchFileInIndirectBlock(file, ext2fs, blocks[12], 1, fileName)
		if found {
			return inodeNum, nil
		}
	}

	if blocks[13] != -1 {
		inodeNum, found := searchFileInIndirectBlock(file, ext2fs, blocks[13], 2, fileName)
		if found {
			return inodeNum, nil
		}
	}

	if blocks[14] != -1 {
		inodeNum, found := searchFileInIndirectBlock(file, ext2fs, blocks[14], 3, fileName)
		if found {
			return inodeNum, nil
		}
	}

	return -1, fmt.Errorf("no se encontró '%s' en el directorio", fileName)
}

// searchFileInIndirectBlock busca un archivo en bloques indirectos
func searchFileInIndirectBlock(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, blockNum int32, level int, fileName string) (int32, bool) {
	if blockNum == -1 {
		return -1, false
	}

	// Leer bloque de apuntadores
	apBlock := &bloques.BloqueApuntador{}
	blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
	err := utilitiesP2.ReadStruct(file, blockOffset, apBlock)
	if err != nil {
		return -1, false
	}

	// Recorrer apuntadores
	for i := 0; i < 16; i++ {
		if apBlock.B_pointers[i] == -1 {
			continue
		}

		if level == 1 {
			// Es un bloque directo
			dirBlock := &bloques.BloqueCarpeta{}
			blockOffset := int64(ext2fs.SuperBlock.S_block_start + (apBlock.B_pointers[i] * ext2fs.SuperBlock.S_block_s))
			err := utilitiesP2.ReadStruct(file, blockOffset, dirBlock)
			if err != nil {
				continue
			}

			// Buscar la entrada en el bloque
			for _, content := range dirBlock.B_content {
				if content.B_inodo != -1 {
					name := stringFromBytes(content.B_name[:])
					if name == fileName {
						return content.B_inodo, true
					}
				}
			}
		} else {
			// Es un bloque indirecto
			inodeNum, found := searchFileInIndirectBlock(file, ext2fs, apBlock.B_pointers[i], level-1, fileName)
			if found {
				return inodeNum, true
			}
		}
	}

	return -1, false
}
