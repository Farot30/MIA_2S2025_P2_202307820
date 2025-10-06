package commandsP2

import (
	BloquesP2 "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt2P2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	sessionP2 "MIA_2S2025_P2_202307820/Session"
	EstructuraarchivocarpetaP2 "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	estructuraArchivoCarpeta "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func MOVE(args []string) {
	// Verificar que hay una sesión activa
	if !sessionP2.IsSessionActive() {
		fmt.Printf("ERROR: No active session. Please log in first.")
		return
	}

	// Parsear parámetros
	params := utilitiesP2.ParseParameters(args)
	sourcePath := strings.Trim(params["path"], "\"")
	destPath := strings.Trim(params["destino"], "\"")

	// Validar parámetros obligatorios
	if sourcePath == "" || destPath == "" {
		fmt.Println("ERROR: Faltan parámetros obligatorios para el comando move: 'path' y 'destino'.")
		return
	}

	// Validar que las rutas sean absolutas
	if !strings.HasPrefix(sourcePath, "/") || !strings.HasPrefix(destPath, "/") {
		fmt.Println("ERROR: Los parámetros 'path' y 'destino' deben comenzar con '/'.")
		return
	}

	// Obtener información de la sesión
	sessionInfo := sessionP2.GetCurrentSession()
	if sessionInfo == nil {
		fmt.Println("ERROR: No se pudo obtener información de la sesión actual.")
		return
	}

	// Obtener la partición montada
	mountedPartition, exist := GetMountedPartition(sessionInfo.PartitionID)
	if !exist {
		fmt.Println("ERROR: La partición no está montada.")
		return
	}

	// Ejecutar el movimiento
	err := moveIN(mountedPartition, sourcePath, destPath, sessionInfo)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}

	fmt.Printf("Movimiento exitoso de '%s' a '%s'\n", sourcePath, destPath)
}

func moveIN(partition MountedPartition, sourcePath, destPath string, sessionInfo *sessionP2.UserSession) error {

	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("no se pudo abrir la partición: %v", err)
	}
	defer file.Close()

	fsType, err := detectFileSystemType(partition)
	if err != nil {
		return fmt.Errorf("no se pudo detectar el tipo de sistema de archivos: %v", err)
	}

	fmt.Printf("[MOVE] Sistema de archivos detectado: %s\n", fsType)

	if fsType == estructuraArchivoCarpeta.EXT2_FILESYSTEM_TYPE {
		return moveInEXT2(file, partition, sourcePath, destPath, sessionInfo)
	} else if fsType == estructuraArchivoCarpeta.EXT3_FILESYSTEM_TYPE {
		return moveInEXT3(file, partition, sourcePath, destPath, sessionInfo)
	}

	return nil
}

// moveInEXT2 maneja el movimiento de archivos/carpetas en sistema EXT2
func moveInEXT2(file *os.File, partition MountedPartition, sourcePath, destPath string, sessionInfo *sessionP2.UserSession) error {

	// Leer el sistema de archivos EXT2
	ext2fs, err := EstructuraExt2P2.ReadEXT2FileSystem(partition.Path, partition.Start)
	if err != nil {
		return fmt.Errorf("no se pudo leer el sistema EXT2: %v", err)
	}

	// Separar directorio padre y nombre del elemento fuente
	sourceDir := filepath.Dir(sourcePath)
	sourceName := filepath.Base(sourcePath)

	// Separar directorio de destino
	destDir := destPath

	// Buscar el directorio padre del elemento fuente
	sourceDirInodeNum, err := findDirectoryInEXT2ForMove(file, ext2fs, sourceDir)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar el directorio padre fuente '%s': %v", sourceDir, err)
	}

	// Buscar el directorio de destino
	destDirInodeNum, err := findDirectoryInEXT2ForMove(file, ext2fs, destDir)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar el directorio destino '%s': %v", destDir, err)
	}

	// Leer inodo del directorio padre fuente
	sourceDirInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+sourceDirInodeNum*ext2fs.SuperBlock.S_inode_s), sourceDirInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer inodo del directorio padre fuente: %v", err)
	}

	// Leer inodo del directorio destino
	destDirInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+destDirInodeNum*ext2fs.SuperBlock.S_inode_s), destDirInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer inodo del directorio destino: %v", err)
	}

	// Verificar permisos de escritura en el directorio destino
	canWrite, err := sessionP2.CheckPermissions(destDirInode, sessionP2.WRITE)
	if err != nil || !canWrite {
		return fmt.Errorf("no tiene permisos de escritura en el directorio destino '%s'", destDir)
	}

	// Buscar el elemento a mover en el directorio fuente
	sourceInodeNum, err := findFileInDirectoryEXT2ForMove(file, ext2fs, sourceDirInode, sourceName)
	if err != nil || sourceInodeNum == -1 {
		return fmt.Errorf("no se pudo encontrar '%s' en '%s'", sourceName, sourceDir)
	}

	// Leer el inodo del elemento fuente
	sourceInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+sourceInodeNum*ext2fs.SuperBlock.S_inode_s), sourceInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer inodo del elemento fuente: %v", err)
	}

	// Verificar permisos de escritura en el elemento fuente
	canWrite, err = sessionP2.CheckPermissions(sourceInode, sessionP2.WRITE)
	if err != nil || !canWrite {
		return fmt.Errorf("no tiene permisos de escritura sobre el elemento '%s'", sourceName)
	}

	// Determinar estrategia según el tipo de elemento
	if sourceInode.Itype == 1 { // Archivo
		return moveFileInEXT2(file, ext2fs, sourceDirInode, destDirInode, sourceName, sourceInodeNum)
	} else if sourceInode.Itype == 0 { // Directorio
		return moveDirectoryInEXT2(file, ext2fs, sourceDirInode, destDirInode, sourceName, sourceInodeNum, sessionInfo)
	}

	return fmt.Errorf("tipo de inodo no reconocido: %d", sourceInode.Itype)
}

func moveInEXT3(file *os.File, partition MountedPartition, sourcePath, destPath string, sessionInfo *sessionP2.UserSession) error {

	// Leer el sistema de archivos EXT3
	ext3fs, err := EstructuraExt3P2.ReadEXT3FileSystem(partition.Path, partition.Start)
	if err != nil {
		return fmt.Errorf("no se pudo leer el sistema EXT3: %v", err)
	}

	err = ext3fs.AddJournalEntry(EstructuraExt3P2.OPERATION_MOVE, sourcePath, fmt.Sprintf("Iniciando movimiento de '%s' a '%s'", sourcePath, destPath))
	if err != nil {
		fmt.Printf("Advertencia: No se pudo registrar inicio de operación en el journal: %s\n", err)
	}

	// Separar directorio padre y nombre del elemento fuente
	sourceDir := filepath.Dir(sourcePath)
	sourceName := filepath.Base(sourcePath)

	// Separar directorio padre y nombre del elemento destino
	destDir := destPath

	// Buscar el directorio padre del elemento fuente
	sourceDirInodeNum, err := findDirectoryInEXT3ForMove(file, ext3fs, sourceDir)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar el directorio padre del elemento fuente '%s': %v", sourceDir, err)
	}

	// Buscar el directorio padre del elemento destino
	destDirInodeNum, err := findDirectoryInEXT3ForMove(file, ext3fs, destDir)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar el directorio padre del elemento destino '%s': %v", destDir, err)
	}

	sourceDirInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+sourceDirInodeNum*ext3fs.SuperBlock.S_inode_s), sourceDirInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer inodo del directorio padre fuente: %v", err)
	}

	destDirInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+destDirInodeNum*ext3fs.SuperBlock.S_inode_s), destDirInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer inodo del directorio padre destino: %v", err)
	}

	// Verificar permisos de escritura en el directorio destino
	canWrite, err := sessionP2.CheckPermissions(destDirInode, sessionP2.WRITE)
	if err != nil || !canWrite {
		return fmt.Errorf("no tiene permisos de escritura sobre el directorio destino '%s'", destDir)
	}

	sourceInodeNum, err := findFileInDirectoryEXT3ForMove(file, ext3fs, sourceDirInode, sourceName)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar el archivo fuente '%s' en el directorio: %v", sourceName, err)
	}

	sourceInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+sourceInodeNum*ext3fs.SuperBlock.S_inode_s), sourceInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer inodo del elemento fuente: %v", err)
	}

	// Verificar permisos de escritura en el elemento fuente
	canWrite, err = sessionP2.CheckPermissions(sourceInode, sessionP2.WRITE)
	if err != nil || !canWrite {
		return fmt.Errorf("no tiene permisos de escritura sobre el elemento '%s'", sourceName)
	}

	// Determinar estrategia según el tipo de elemento
	if sourceInode.Itype == 1 { // Archivo
		return moveFileInEXT3(file, ext3fs, sourceDirInode, destDirInode, sourceName, sourceInodeNum)
	} else if sourceInode.Itype == 0 { // Directorio
		return moveDirectoryInEXT3(file, ext3fs, sourceDirInode, destDirInode, sourceName, sourceInodeNum, sessionInfo)
	}
	return nil
}

// moveFileInEXT2 mueve un archivo (cambio de referencias)
func moveFileInEXT2(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, sourceDirInode, destDirInode *EstructuraarchivocarpetaP2.Inodo, fileName string, fileInodeNum int32) error {
	fmt.Printf("[MOVE-FILE] Moviendo archivo '%s' mediante cambio de referencias\n", fileName)

	// 1. Agregar entrada en el directorio destino
	err := addEntryToDirectory(file, ext2fs, destDirInode, fileName, fileInodeNum)
	if err != nil {
		return fmt.Errorf("no se pudo agregar entrada al directorio destino: %v", err)
	}

	// 2. Remover entrada del directorio fuente
	err = removeEntryFromDirectoryMove(file, ext2fs, sourceDirInode, fileName)
	if err != nil {
		return fmt.Errorf("no se pudo remover entrada del directorio fuente: %v", err)
	}

	fmt.Printf("[MOVE-FILE] Archivo movido exitosamente\n")
	return nil
}

// moveDirectoryInEXT2 mueve un directorio usando estrategia de preservación de contenido
func moveDirectoryInEXT2(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, sourceDirInode, destDirInode *EstructuraarchivocarpetaP2.Inodo, dirName string, dirInodeNum int32, sessionInfo *sessionP2.UserSession) error {
	fmt.Printf("[MOVE-DIR] Moviendo directorio '%s' usando estrategia de preservación\n", dirName)

	// Leer el inodo del directorio a mover
	moveInode := &EstructuraarchivocarpetaP2.Inodo{}
	err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+dirInodeNum*ext2fs.SuperBlock.S_inode_s), moveInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer inodo del directorio: %v", err)
	}

	// 1. Guardar todo el contenido del directorio recursivamente
	dirContent, err := saveDirectoryContent(file, ext2fs, moveInode, dirName)
	if err != nil {
		return fmt.Errorf("no se pudo guardar contenido del directorio: %v", err)
	}

	// 2. Recrear el directorio en el destino
	newDirInodeNum, err := createDirectoryWithContent(file, ext2fs, destDirInode, dirContent, sessionInfo)
	if err != nil {
		return fmt.Errorf("no se pudo recrear directorio en destino: %v", err)
	}

	// 3. Limpiar el directorio original (liberar bloques e inodos)
	err = cleanupDirectoryRecursively(file, ext2fs, moveInode, dirInodeNum)
	if err != nil {
		return fmt.Errorf("no se pudo limpiar directorio original: %v", err)
	}

	// 4. Remover entrada del directorio fuente
	err = removeEntryFromDirectoryMove(file, ext2fs, sourceDirInode, dirName)
	if err != nil {
		return fmt.Errorf("no se pudo remover entrada del directorio fuente: %v", err)
	}

	fmt.Printf("[MOVE-DIR] Directorio movido exitosamente (nuevo inodo: %d)\n", newDirInodeNum)
	return nil
}

func moveFileInEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, sourceDirInode, destDirInode *EstructuraarchivocarpetaP2.Inodo, fileName string, fileInodeNum int32) error {
	fmt.Printf("[MOVE-FILE-EXT3] Moviendo archivo '%s' mediante cambio de referencias\n", fileName)

	// 1. Agregar entrada en el directorio destino
	err := addEntryToDirectoryEXT3(file, ext3fs, destDirInode, fileName, fileInodeNum)
	if err != nil {
		return fmt.Errorf("no se pudo agregar entrada al directorio destino: %v", err)
	}

	// 2. Remover entrada del directorio fuente
	err = removeEntryFromDirectoryMoveEXT3(file, ext3fs, sourceDirInode, fileName)
	if err != nil {
		return fmt.Errorf("no se pudo remover entrada del directorio fuente: %v", err)
	}

	err = ext3fs.AddJournalEntry(EstructuraExt3P2.OPERATION_MOVE, fileName, "Archivo movido exitosamente")
	if err != nil {
		return fmt.Errorf("no se pudo agregar entrada al journal: %v", err)
	}

	fmt.Printf("[MOVE-FILE-EXT3] Archivo movido exitosamente\n")
	return nil
}

func moveDirectoryInEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, sourceDirInode, destDirInode *EstructuraarchivocarpetaP2.Inodo, dirName string, dirInodeNum int32, sessionInfo *sessionP2.UserSession) error {
	fmt.Printf("[MOVE-DIR-EXT3] Moviendo directorio '%s' usando estrategia de preservación\n", dirName)

	// Leer el inodo del directorio a mover
	moveInode := &EstructuraarchivocarpetaP2.Inodo{}
	err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+dirInodeNum*ext3fs.SuperBlock.S_inode_s), moveInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer inodo del directorio: %v", err)
	}

	// 1. Guardar todo el contenido del directorio recursivamente
	dirContent, err := saveDirectoryContentEXT3(file, ext3fs, moveInode, dirName)
	if err != nil {
		return fmt.Errorf("no se pudo guardar contenido del directorio: %v", err)
	}

	// 2. Recrear el directorio en el destino
	newDirInodeNum, err := createDirectoryWithContentEXT3(file, ext3fs, destDirInode, dirContent, sessionInfo)
	if err != nil {
		return fmt.Errorf("no se pudo recrear directorio en destino: %v", err)
	}

	// 3. Limpiar el directorio original (liberar bloques e inodos)
	err = cleanupDirectoryRecursivelyEXT3(file, ext3fs, moveInode, dirInodeNum)
	if err != nil {
		return fmt.Errorf("no se pudo limpiar directorio original: %v", err)
	}

	// 4. Remover entrada del directorio fuente
	err = removeEntryFromDirectoryMoveEXT3(file, ext3fs, sourceDirInode, dirName)
	if err != nil {
		return fmt.Errorf("no se pudo remover entrada del directorio fuente: %v", err)
	}

	err = ext3fs.AddJournalEntry(EstructuraExt3P2.OPERATION_MOVE, dirName, "Directorio movido exitosamente")
	if err != nil {
		return fmt.Errorf("no se pudo agregar entrada al journal: %v", err)
	}

	fmt.Printf("[MOVE-DIR-EXT3] Directorio movido exitosamente (nuevo inodo: %d)\n", newDirInodeNum)
	return nil
}

// DirectoryContent representa el contenido completo de un directorio
type DirectoryContent struct {
	Name        string
	IsDirectory bool
	Permissions [3]byte
	UID         int32
	GID         int32
	Content     []byte                       // Para archivos
	Children    map[string]*DirectoryContent // Para directorios
}

// saveDirectoryContent guarda recursivamente todo el contenido de un directorio
func saveDirectoryContent(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, dirName string) (*DirectoryContent, error) {
	fmt.Printf("[SAVE-CONTENT] Guardando contenido del directorio '%s'\n", dirName)

	dirContent := &DirectoryContent{
		Name:        dirName,
		IsDirectory: true,
		Permissions: dirInode.Iperm,
		UID:         dirInode.Iuid,
		GID:         dirInode.Igid,
		Children:    make(map[string]*DirectoryContent),
	}

	// Recorrer los bloques del directorio
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		block := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+blockNum*ext2fs.SuperBlock.S_block_s), block)
		if err != nil {
			return nil, fmt.Errorf("error leyendo bloque del directorio: %v", err)
		}

		// Procesar cada entrada del bloque
		for j := 0; j < 4; j++ {
			if !block.IsEntryEmpty(j) {
				entryName, entryInodeNum, _ := block.GetEntry(j)

				// Saltar entradas . y ..
				if entryName == "." || entryName == ".." {
					continue
				}

				// Leer el inodo de la entrada
				entryInode := &EstructuraarchivocarpetaP2.Inodo{}
				err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+entryInodeNum*ext2fs.SuperBlock.S_inode_s), entryInode)
				if err != nil {
					fmt.Printf("[WARNING] No se pudo leer inodo de '%s': %v\n", entryName, err)
					continue
				}

				if entryInode.Itype == 1 { // Archivo
					// Leer contenido del archivo
					fileContent, err := ReadFileContentCorrect(file, ext2fs, entryInode)
					if err != nil {
						fmt.Printf("[WARNING] No se pudo leer contenido de archivo '%s': %v\n", entryName, err)
						fileContent = []byte{} // Contenido vacío como fallback
					}

					dirContent.Children[entryName] = &DirectoryContent{
						Name:        entryName,
						IsDirectory: false,
						Permissions: entryInode.Iperm,
						UID:         entryInode.Iuid,
						GID:         entryInode.Igid,
						Content:     fileContent,
					}

					fmt.Printf("[SAVE-FILE] Archivo '%s' guardado (%d bytes)\n", entryName, len(fileContent))

				} else if entryInode.Itype == 0 { // Directorio
					// Guardar recursivamente
					subDirContent, err := saveDirectoryContent(file, ext2fs, entryInode, entryName)
					if err != nil {
						fmt.Printf("[WARNING] No se pudo guardar subdirectorio '%s': %v\n", entryName, err)
						continue
					}

					dirContent.Children[entryName] = subDirContent
					fmt.Printf("[SAVE-DIR] Directorio '%s' guardado recursivamente\n", entryName)
				}
			}
		}
	}

	fmt.Printf("[SAVE-CONTENT] Contenido del directorio '%s' guardado (%d elementos)\n", dirName, len(dirContent.Children))
	return dirContent, nil
}

func saveDirectoryContentEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, dirName string) (*DirectoryContent, error) {
	fmt.Printf("[SAVE-CONTENT-EXT3] Guardando contenido del directorio '%s'\n", dirName)

	dirContent := &DirectoryContent{
		Name:        dirName,
		IsDirectory: true,
		Permissions: dirInode.Iperm,
		UID:         dirInode.Iuid,
		GID:         dirInode.Igid,
		Children:    make(map[string]*DirectoryContent),
	}

	// Recorrer los bloques del directorio
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		block := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+blockNum*ext3fs.SuperBlock.S_block_s), block)
		if err != nil {
			return nil, fmt.Errorf("error leyendo bloque del directorio: %v", err)
		}

		// Procesar cada entrada del bloque
		for j := 0; j < 4; j++ {
			if !block.IsEntryEmpty(j) {
				entryName, entryInodeNum, _ := block.GetEntry(j)

				// Saltar entradas . y ..
				if entryName == "." || entryName == ".." {
					continue
				}

				// Leer el inodo de la entrada
				entryInode := &EstructuraarchivocarpetaP2.Inodo{}
				err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+entryInodeNum*ext3fs.SuperBlock.S_inode_s), entryInode)
				if err != nil {
					fmt.Printf("[WARNING] No se pudo leer inodo de '%s': %v\n", entryName, err)
					continue
				}

				if entryInode.Itype == 1 { // Archivo
					// Leer contenido del archivo
					fileContent, err := ReadFileContentCorrectEXT3(file, ext3fs, entryInode)
					if err != nil {
						fmt.Printf("[WARNING] No se pudo leer contenido de archivo '%s': %v\n", entryName, err)
						fileContent = []byte{} // Contenido vacío como fallback
					}

					dirContent.Children[entryName] = &DirectoryContent{
						Name:        entryName,
						IsDirectory: false,
						Permissions: entryInode.Iperm,
						UID:         entryInode.Iuid,
						GID:         entryInode.Igid,
						Content:     fileContent,
					}

					fmt.Printf("[SAVE-FILE-EXT3] Archivo '%s' guardado (%d bytes)\n", entryName, len(fileContent))

				} else if entryInode.Itype == 0 { // Directorio
					// Guardar recursivamente
					subDirContent, err := saveDirectoryContentEXT3(file, ext3fs, entryInode, entryName)
					if err != nil {
						fmt.Printf("[WARNING] No se pudo guardar subdirectorio '%s': %v\n", entryName, err)
						continue
					}

					dirContent.Children[entryName] = subDirContent
					fmt.Printf("[SAVE-DIR-EXT3] Directorio '%s' guardado recursivamente\n", entryName)
				}
			}
		}
	}

	fmt.Printf("[SAVE-CONTENT-EXT3] Contenido del directorio '%s' guardado (%d elementos)\n", dirName, len(dirContent.Children))
	return dirContent, nil
}

// createDirectoryWithContent recrea un directorio con todo su contenido
func createDirectoryWithContent(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, parentInode *EstructuraarchivocarpetaP2.Inodo, dirContent *DirectoryContent, sessionInfo *sessionP2.UserSession) (int32, error) {
	fmt.Printf("[CREATE-CONTENT] Recreando directorio '%s' con contenido\n", dirContent.Name)

	// Crear el directorio base
	newDirInodeNum, err := createBasicDirectory(file, ext2fs, parentInode, dirContent.Name, dirContent.UID, dirContent.GID, dirContent.Permissions)
	if err != nil {
		return -1, fmt.Errorf("no se pudo crear directorio base: %v", err)
	}

	// Leer el inodo del nuevo directorio
	newDirInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+newDirInodeNum*ext2fs.SuperBlock.S_inode_s), newDirInode)
	if err != nil {
		return -1, fmt.Errorf("no se pudo leer inodo del nuevo directorio: %v", err)
	}

	// Recrear todo el contenido
	for name, content := range dirContent.Children {
		if content.IsDirectory {
			// Recrear subdirectorio recursivamente
			_, err := createDirectoryWithContent(file, ext2fs, newDirInode, content, sessionInfo)
			if err != nil {
				fmt.Printf("[WARNING] No se pudo recrear subdirectorio '%s': %v\n", name, err)
			}
		} else {
			// Recrear archivo
			err := createFileWithContent(file, ext2fs, newDirInode, content)
			if err != nil {
				fmt.Printf("[WARNING] No se pudo recrear archivo '%s': %v\n", name, err)
			}
		}
	}

	fmt.Printf("[CREATE-CONTENT] Directorio '%s' recreado exitosamente\n", dirContent.Name)
	return newDirInodeNum, nil
}

func createDirectoryWithContentEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, parentInode *EstructuraarchivocarpetaP2.Inodo, dirContent *DirectoryContent, sessionInfo *sessionP2.UserSession) (int32, error) {
	fmt.Printf("[CREATE-CONTENT-EXT3] Recreando directorio '%s' con contenido\n", dirContent.Name)

	// Crear el directorio base
	newDirInodeNum, err := createBasicDirectoryEXT3(file, ext3fs, parentInode, dirContent.Name, dirContent.UID, dirContent.GID, dirContent.Permissions)
	if err != nil {
		return -1, fmt.Errorf("no se pudo crear directorio base: %v", err)
	}

	newDirInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+newDirInodeNum*ext3fs.SuperBlock.S_inode_s), newDirInode)
	if err != nil {
		return -1, fmt.Errorf("no se pudo leer inodo del nuevo directorio: %v", err)
	}

	// Recrear todo el contenido
	for name, content := range dirContent.Children {
		if content.IsDirectory {
			// Recrear subdirectorio recursivamente
			_, err := createDirectoryWithContentEXT3(file, ext3fs, newDirInode, content, sessionInfo)
			if err != nil {
				fmt.Printf("[WARNING] No se pudo recrear subdirectorio '%s': %v\n", name, err)
			}
		} else {
			// Recrear archivo
			err := createFileWithContentEXT3(file, ext3fs, newDirInode, content)
			if err != nil {
				fmt.Printf("[WARNING] No se pudo recrear archivo '%s': %v\n", name, err)
			}
		}
	}

	fmt.Printf("[CREATE-CONTENT-EXT3] Directorio '%s' recreado exitosamente\n", dirContent.Name)
	return newDirInodeNum, nil
}

// findDirectoryInEXT2ForMove busca un directorio en el sistema EXT2 y devuelve su número de inodo
func findDirectoryInEXT2ForMove(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, path string) (int32, error) {
	// Caso especial: directorio raíz
	if path == "/" {
		return 0, nil
	}

	// Normalizar la ruta
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return 0, nil
	}

	// Dividir la ruta en componentes
	components := strings.Split(strings.TrimPrefix(path, "/"), "/")
	currentInodeNum := int32(0) // Comenzar desde el directorio raíz

	// Recorrer cada componente de la ruta
	for _, component := range components {
		if component == "" {
			continue
		}

		// Leer el inodo del directorio actual
		currentInode := &EstructuraarchivocarpetaP2.Inodo{}
		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+currentInodeNum*ext2fs.SuperBlock.S_inode_s), currentInode)
		if err != nil {
			return -1, fmt.Errorf("error leyendo inodo del directorio: %v", err)
		}

		// Verificar que sea un directorio
		if currentInode.Itype != 0 {
			return -1, fmt.Errorf("'%s' no es un directorio", component)
		}

		// Buscar el componente en el directorio actual
		nextInodeNum, err := findFileInDirectoryEXT2ForMove(file, ext2fs, currentInode, component)
		if err != nil || nextInodeNum == -1 {
			return -1, fmt.Errorf("no se encontró '%s' en el directorio", component)
		}

		currentInodeNum = nextInodeNum
	}

	return currentInodeNum, nil
}

func findDirectoryInEXT3ForMove(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path string) (int32, error) {
	// Implementación similar a findFileInDirectoryEXT2ForMove, adaptada para EXT3
	if path == "/" {
		return 0, nil
	}

	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return 0, nil
	}

	components := strings.Split(strings.TrimPrefix(path, "/"), "/")
	currentInodeNum := int32(0) // Comenzar desde el directorio raíz

	for _, component := range components {
		if component == "" {
			continue
		}

		currentInode := &EstructuraarchivocarpetaP2.Inodo{}
		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+currentInodeNum*ext3fs.SuperBlock.S_inode_s), currentInode)
		if err != nil {
			return -1, fmt.Errorf("error leyendo inodo del directorio: %v", err)
		}

		if currentInode.Itype != 0 {
			return -1, fmt.Errorf("'%s' no es un directorio", component)
		}

		nextInodeNum, err := findFileInDirectoryEXT3ForMove(file, ext3fs, currentInode, component)
		if err != nil || nextInodeNum == -1 {
			return -1, fmt.Errorf("no se encontró '%s' en el directorio", component)
		}

		currentInodeNum = nextInodeNum
	}

	return currentInodeNum, nil
}

func findFileInDirectoryEXT3ForMove(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, fileName string) (int32, error) {
	// Implementación similar a findFileInDirectoryEXT2ForMove, adaptada para EXT3
	if dirInode.Itype != 0 {
		return -1, fmt.Errorf("el inodo no es un directorio")
	}

	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		block := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+blockNum*ext3fs.SuperBlock.S_block_s), block)
		if err != nil {
			continue
		}

		for j := 0; j < 4; j++ {
			if !block.IsEntryEmpty(j) {
				entryName, entryInodeNum, _ := block.GetEntry(j)
				if entryName == fileName {
					return entryInodeNum, nil
				}
			}
		}
	}

	return -1, fmt.Errorf("no se encontró '%s' en el directorio", fileName)
}

// findFileInDirectoryEXT2ForMove busca un archivo o directorio en un directorio específico
func findFileInDirectoryEXT2ForMove(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, fileName string) (int32, error) {
	// Verificar que sea un directorio
	if dirInode.Itype != 0 {
		return -1, fmt.Errorf("el inodo no es un directorio")
	}

	// Buscar en bloques directos
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		block := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+blockNum*ext2fs.SuperBlock.S_block_s), block)
		if err != nil {
			continue
		}

		// Buscar en las entradas del bloque
		for j := 0; j < 4; j++ {
			if !block.IsEntryEmpty(j) {
				entryName, entryInodeNum, _ := block.GetEntry(j)
				if entryName == fileName {
					return entryInodeNum, nil
				}
			}
		}
	}

	return -1, fmt.Errorf("no se encontró '%s' en el directorio", fileName)
}

// addEntryToDirectory agrega una entrada a un directorio
func addEntryToDirectory(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, entryName string, entryInodeNum int32) error {
	// Buscar espacio libre en los bloques existentes
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		block := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+blockNum*ext2fs.SuperBlock.S_block_s), block)
		if err != nil {
			continue
		}

		// Buscar una entrada vacía
		if block.AddEntry(entryName, entryInodeNum) {
			// Escribir el bloque actualizado
			err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+blockNum*ext2fs.SuperBlock.S_block_s), block)
			if err != nil {
				return fmt.Errorf("error escribiendo bloque de directorio: %v", err)
			}
			return nil
		}
	}

	// Si no hay espacio, crear un nuevo bloque
	newBlockNum, err := EstructuraExt2P2.FindFreeBlockEXT2(file, ext2fs)
	if err != nil || newBlockNum == -1 {
		return fmt.Errorf("no se pudo encontrar un bloque libre: %v", err)
	}

	// Crear nuevo bloque de directorio
	newBlock := BloquesP2.NewBloqueCarpeta()
	newBlock.AddEntry(entryName, entryInodeNum)

	// Escribir el nuevo bloque
	err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+newBlockNum*ext2fs.SuperBlock.S_block_s), newBlock)
	if err != nil {
		return fmt.Errorf("error escribiendo nuevo bloque de directorio: %v", err)
	}

	// Asignar el bloque al directorio
	for i := 0; i < 12; i++ {
		if dirInode.Iblock[i] == -1 {
			dirInode.Iblock[i] = newBlockNum
			break
		}
	}

	return nil
}

func addEntryToDirectoryEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, entryName string, entryInodeNum int32) error {
	// Implementación similar a addEntryToDirectory, adaptada para EXT3
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		block := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+blockNum*ext3fs.SuperBlock.S_block_s), block)
		if err != nil {
			continue
		}

		if block.AddEntry(entryName, entryInodeNum) {
			err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+blockNum*ext3fs.SuperBlock.S_block_s), block)
			if err != nil {
				return fmt.Errorf("error escribiendo bloque de directorio: %v", err)
			}
			return nil
		}
	}

	newBlockNum, err := findFreeBlockEXT3(file, ext3fs)
	if err != nil || newBlockNum == -1 {
		return fmt.Errorf("no se pudo encontrar un bloque libre: %v", err)
	}

	newBlock := BloquesP2.NewBloqueCarpeta()
	newBlock.AddEntry(entryName, entryInodeNum)

	err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+newBlockNum*ext3fs.SuperBlock.S_block_s), newBlock)
	if err != nil {
		return fmt.Errorf("error escribiendo nuevo bloque de directorio: %v", err)
	}

	for i := 0; i < 12; i++ {
		if dirInode.Iblock[i] == -1 {
			dirInode.Iblock[i] = newBlockNum
			break
		}
	}

	return nil
}

// removeEntryFromDirectoryMove elimina una entrada de un directorio
func removeEntryFromDirectoryMove(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, entryName string) error {
	// Buscar y eliminar la entrada en los bloques del directorio
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		block := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+blockNum*ext2fs.SuperBlock.S_block_s), block)
		if err != nil {
			continue
		}

		// Buscar la entrada por nombre
		entryIndex, _, found := block.FindEntryByName(entryName)
		if found {
			// Limpiar la entrada
			block.B_content[entryIndex].Clear()

			// Escribir el bloque actualizado
			err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+blockNum*ext2fs.SuperBlock.S_block_s), block)
			if err != nil {
				return fmt.Errorf("error escribiendo bloque de directorio: %v", err)
			}
			return nil
		}
	}

	return fmt.Errorf("no se encontró la entrada '%s' en el directorio", entryName)
}

func removeEntryFromDirectoryMoveEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, entryName string) error {
	// Implementación similar a removeEntryFromDirectoryMove, adaptada para EXT3
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		block := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+blockNum*ext3fs.SuperBlock.S_block_s), block)
		if err != nil {
			continue
		}

		entryIndex, _, found := block.FindEntryByName(entryName)
		if found {
			block.B_content[entryIndex].Clear()

			err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+blockNum*ext3fs.SuperBlock.S_block_s), block)
			if err != nil {
				return fmt.Errorf("error escribiendo bloque de directorio: %v", err)
			}
			return nil
		}
	}

	return fmt.Errorf("no se encontró la entrada '%s' en el directorio", entryName)
}

// createBasicDirectory crea un directorio básico con propiedades específicas
func createBasicDirectory(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, parentInode *EstructuraarchivocarpetaP2.Inodo, dirName string, uid, gid int32, permissions [3]byte) (int32, error) {
	// Buscar un inodo libre
	newInodeNum, err := findFreeInodeEXT2Move(file, ext2fs)
	if err != nil {
		return -1, fmt.Errorf("no se pudo encontrar un inodo libre: %v", err)
	}

	// Buscar un bloque libre para el directorio
	newBlockNum, err := EstructuraExt2P2.FindFreeBlockEXT2(file, ext2fs)
	if err != nil || newBlockNum == -1 {
		return -1, fmt.Errorf("no se pudo encontrar un bloque libre: %v", err)
	}

	// Crear el inodo del directorio
	newDirInode := &EstructuraarchivocarpetaP2.Inodo{}
	newDirInode.Iuid = uid
	newDirInode.Igid = gid
	newDirInode.Is = 0
	newDirInode.Iatime = EstructuraarchivocarpetaP2.GetCurrentTimestamp()
	newDirInode.Ictime = EstructuraarchivocarpetaP2.GetCurrentTimestamp()
	newDirInode.Imtime = EstructuraarchivocarpetaP2.GetCurrentTimestamp()
	newDirInode.Itype = 0 // Directorio
	newDirInode.Iperm = permissions

	// Inicializar bloques
	for i := 0; i < 16; i++ {
		newDirInode.Iblock[i] = -1
	}
	newDirInode.Iblock[0] = newBlockNum

	// Crear el bloque del directorio con entradas . y ..
	dirBlock := BloquesP2.NewBloqueCarpeta()
	dirBlock.AddEntry(".", newInodeNum)                                   // Entrada actual
	dirBlock.AddEntry("..", getParentInodeNum(file, ext2fs, parentInode)) // Entrada padre

	// Escribir el bloque del directorio
	err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+newBlockNum*ext2fs.SuperBlock.S_block_s), dirBlock)
	if err != nil {
		return -1, fmt.Errorf("error escribiendo bloque del directorio: %v", err)
	}

	// Escribir el inodo del directorio
	err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_inode_start+newInodeNum*ext2fs.SuperBlock.S_inode_s), newDirInode)
	if err != nil {
		return -1, fmt.Errorf("error escribiendo inodo del directorio: %v", err)
	}

	// Agregar entrada al directorio padre
	err = addEntryToDirectory(file, ext2fs, parentInode, dirName, newInodeNum)
	if err != nil {
		return -1, fmt.Errorf("error agregando entrada al directorio padre: %v", err)
	}

	return newInodeNum, nil
}

func createBasicDirectoryEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, parentInode *EstructuraarchivocarpetaP2.Inodo, dirName string, uid, gid int32, permissions [3]byte) (int32, error) {
	// Implementación similar a createBasicDirectory, adaptada para EXT3
	newInodeNum, err := findFreeInodeEXT3Move(file, ext3fs)
	if err != nil {
		return -1, fmt.Errorf("no se pudo encontrar un inodo libre: %v", err)
	}

	newBlockNum, err := findFreeBlockEXT3(file, ext3fs)
	if err != nil || newBlockNum == -1 {
		return -1, fmt.Errorf("no se pudo encontrar un bloque libre: %v", err)
	}

	newDirInode := &EstructuraarchivocarpetaP2.Inodo{}
	newDirInode.Iuid = uid
	newDirInode.Igid = gid
	newDirInode.Is = 0
	newDirInode.Iatime = EstructuraarchivocarpetaP2.GetCurrentTimestamp()
	newDirInode.Ictime = EstructuraarchivocarpetaP2.GetCurrentTimestamp()
	newDirInode.Imtime = EstructuraarchivocarpetaP2.GetCurrentTimestamp()
	newDirInode.Itype = 0 // Directorio
	newDirInode.Iperm = permissions

	for i := 0; i < 16; i++ {
		newDirInode.Iblock[i] = -1
	}
	newDirInode.Iblock[0] = newBlockNum

	dirBlock := BloquesP2.NewBloqueCarpeta()
	dirBlock.AddEntry(".", newInodeNum)
	dirBlock.AddEntry("..", getParentInodeNumEXT3(file, ext3fs, parentInode))

	err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+newBlockNum*ext3fs.SuperBlock.S_block_s), dirBlock)
	if err != nil {
		return -1, fmt.Errorf("error escribiendo bloque del directorio: %v", err)
	}

	err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_inode_start+newInodeNum*ext3fs.SuperBlock.S_inode_s), newDirInode)
	if err != nil {
		return -1, fmt.Errorf("error escribiendo inodo del directorio: %v", err)
	}

	err = addEntryToDirectoryEXT3(file, ext3fs, parentInode, dirName, newInodeNum)
	if err != nil {
		return -1, fmt.Errorf("error agregando entrada al directorio padre: %v", err)
	}

	return newInodeNum, nil
}

// getParentInodeNum obtiene el número de inodo del directorio padre
func getParentInodeNum(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, parentInode *EstructuraarchivocarpetaP2.Inodo) int32 {
	// Si es el directorio raíz, el padre es él mismo
	if parentInode.Iblock[0] != -1 {
		// Leer el primer bloque para verificar si tiene entrada ".."
		block := &BloquesP2.BloqueCarpeta{}
		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+parentInode.Iblock[0]*ext2fs.SuperBlock.S_block_s), block)
		if err == nil {
			// Buscar la entrada ".."
			for i := 0; i < 4; i++ {
				if !block.IsEntryEmpty(i) {
					entryName, entryInodeNum, _ := block.GetEntry(i)
					if entryName == ".." {
						return entryInodeNum
					}
				}
			}
		}
	}
	return 0 // Por defecto, el directorio raíz
}

func getParentInodeNumEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, parentInode *EstructuraarchivocarpetaP2.Inodo) int32 {
	// Implementación similar a getParentInodeNum, adaptada para EXT3
	if parentInode.Iblock[0] != -1 {
		block := &BloquesP2.BloqueCarpeta{}
		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+parentInode.Iblock[0]*ext3fs.SuperBlock.S_block_s), block)
		if err == nil {
			for i := 0; i < 4; i++ {
				if !block.IsEntryEmpty(i) {
					entryName, entryInodeNum, _ := block.GetEntry(i)
					if entryName == ".." {
						return entryInodeNum
					}
				}
			}
		}
	}
	return 0
}

// createFileWithContent crea un archivo con contenido específico
func createFileWithContent(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, parentInode *EstructuraarchivocarpetaP2.Inodo, fileContent *DirectoryContent) error {
	// Buscar un inodo libre
	newInodeNum, err := findFreeInodeEXT2Move(file, ext2fs)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar un inodo libre: %v", err)
	}

	// Crear el inodo del archivo
	newFileInode := &EstructuraarchivocarpetaP2.Inodo{}
	newFileInode.Iuid = fileContent.UID
	newFileInode.Igid = fileContent.GID
	newFileInode.Is = int32(len(fileContent.Content))
	newFileInode.Iatime = EstructuraarchivocarpetaP2.GetCurrentTimestamp()
	newFileInode.Ictime = EstructuraarchivocarpetaP2.GetCurrentTimestamp()
	newFileInode.Imtime = EstructuraarchivocarpetaP2.GetCurrentTimestamp()
	newFileInode.Itype = 1 // Archivo
	newFileInode.Iperm = fileContent.Permissions

	// Inicializar bloques
	for i := 0; i < 16; i++ {
		newFileInode.Iblock[i] = -1
	}

	// Escribir el contenido usando la función existente
	err = WriteFileContentCorrect(file, ext2fs, newFileInode, fileContent.Content)
	if err != nil {
		return fmt.Errorf("error escribiendo contenido del archivo: %v", err)
	}

	// Escribir el inodo del archivo
	err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_inode_start+newInodeNum*ext2fs.SuperBlock.S_inode_s), newFileInode)
	if err != nil {
		return fmt.Errorf("error escribiendo inodo del archivo: %v", err)
	}

	// Agregar entrada al directorio padre
	err = addEntryToDirectory(file, ext2fs, parentInode, fileContent.Name, newInodeNum)
	if err != nil {
		return fmt.Errorf("error agregando entrada al directorio padre: %v", err)
	}

	return nil
}

func createFileWithContentEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, parentInode *EstructuraarchivocarpetaP2.Inodo, fileContent *DirectoryContent) error {
	// Implementación similar a createFileWithContent, adaptada para EXT3
	newInodeNum, err := findFreeInodeEXT3Move(file, ext3fs)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar un inodo libre: %v", err)
	}

	newFileInode := &EstructuraarchivocarpetaP2.Inodo{}
	newFileInode.Iuid = fileContent.UID
	newFileInode.Igid = fileContent.GID
	newFileInode.Is = int32(len(fileContent.Content))
	newFileInode.Iatime = EstructuraarchivocarpetaP2.GetCurrentTimestamp()
	newFileInode.Ictime = EstructuraarchivocarpetaP2.GetCurrentTimestamp()
	newFileInode.Imtime = EstructuraarchivocarpetaP2.GetCurrentTimestamp()
	newFileInode.Itype = 1 // Archivo
	newFileInode.Iperm = fileContent.Permissions

	for i := 0; i < 16; i++ {
		newFileInode.Iblock[i] = -1
	}

	err = WriteFileContentCorrectEXT3(file, ext3fs, newFileInode, fileContent.Content)
	if err != nil {
		return fmt.Errorf("error escribiendo contenido del archivo: %v", err)
	}

	err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_inode_start+newInodeNum*ext3fs.SuperBlock.S_inode_s), newFileInode)
	if err != nil {
		return fmt.Errorf("error escribiendo inodo del archivo: %v", err)
	}

	err = addEntryToDirectoryEXT3(file, ext3fs, parentInode, fileContent.Name, newInodeNum)
	if err != nil {
		return fmt.Errorf("error agregando entrada al directorio padre: %v", err)
	}

	return nil
}

// cleanupDirectoryRecursively limpia un directorio y libera todos sus recursos
func cleanupDirectoryRecursively(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, dirInodeNum int32) error {
	// Liberar todos los bloques del directorio
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]

		// Marcar el bloque como libre en el bitmap
		err := markBlockAsFreeMove(file, ext2fs, blockNum)
		if err != nil {
			fmt.Printf("[WARNING] No se pudo liberar bloque %d: %v\n", blockNum, err)
		}
	}

	// Marcar el inodo como libre en el bitmap
	err := markInodeAsFreeMove(file, ext2fs, dirInodeNum)
	if err != nil {
		return fmt.Errorf("error liberando inodo %d: %v", dirInodeNum, err)
	}

	return nil
}

func cleanupDirectoryRecursivelyEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, dirInodeNum int32) error {
	// Implementación similar a cleanupDirectoryRecursively, adaptada para EXT3
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]

		err := markBlockAsFreeMoveEXT3(file, ext3fs, blockNum)
		if err != nil {
			fmt.Printf("[WARNING] No se pudo liberar bloque %d: %v\n", blockNum, err)
		}
	}

	err := markInodeAsFreeMoveEXT3(file, ext3fs, dirInodeNum)
	if err != nil {
		return fmt.Errorf("error liberando inodo %d: %v", dirInodeNum, err)
	}

	return nil
}

// markBlockAsFreeMove marca un bloque como libre en el bitmap
func markBlockAsFreeMove(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, blockNum int32) error {
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
		return fmt.Errorf("error leyendo bitmap de bloques: %v", err)
	}

	// Limpiar el bit (marcar como libre)
	byteValue[0] &= ^(1 << bitPosition)

	// Escribir el byte modificado
	_, err = file.WriteAt(byteValue[:], bitmapOffset)
	if err != nil {
		return fmt.Errorf("error escribiendo bitmap de bloques: %v", err)
	}

	return nil
}

func markBlockAsFreeMoveEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, blockNum int32) error {
	if blockNum < 0 {
		return nil
	}

	bytePosition := blockNum / 8
	bitPosition := blockNum % 8

	bitmapOffset := int64(ext3fs.SuperBlock.S_bm_block_start + bytePosition)
	var byteValue [1]byte
	_, err := file.ReadAt(byteValue[:], bitmapOffset)
	if err != nil {
		return fmt.Errorf("error leyendo bitmap de bloques: %v", err)
	}

	byteValue[0] &= ^(1 << bitPosition)

	_, err = file.WriteAt(byteValue[:], bitmapOffset)
	if err != nil {
		return fmt.Errorf("error escribiendo bitmap de bloques: %v", err)
	}

	return nil
}

// markInodeAsFreeMove marca un inodo como libre en el bitmap
func markInodeAsFreeMove(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, inodeNum int32) error {
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
		return fmt.Errorf("error leyendo bitmap de inodos: %v", err)
	}

	// Limpiar el bit (marcar como libre)
	byteValue[0] &= ^(1 << bitPosition)

	// Escribir el byte modificado
	_, err = file.WriteAt(byteValue[:], bitmapOffset)
	if err != nil {
		return fmt.Errorf("error escribiendo bitmap de inodos: %v", err)
	}

	return nil
}

func markInodeAsFreeMoveEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, inodeNum int32) error {
	if inodeNum < 0 {
		return nil
	}

	bytePosition := inodeNum / 8
	bitPosition := inodeNum % 8

	bitmapOffset := int64(ext3fs.SuperBlock.S_bm_inode_start + bytePosition)
	var byteValue [1]byte
	_, err := file.ReadAt(byteValue[:], bitmapOffset)
	if err != nil {
		return fmt.Errorf("error leyendo bitmap de inodos: %v", err)
	}

	byteValue[0] &= ^(1 << bitPosition)

	_, err = file.WriteAt(byteValue[:], bitmapOffset)
	if err != nil {
		return fmt.Errorf("error escribiendo bitmap de inodos: %v", err)
	}

	return nil
}

// findFreeInodeEXT2Move busca un inodo libre en el sistema EXT2
func findFreeInodeEXT2Move(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem) (int32, error) {
	// Leer el bitmap de inodos
	bitmapSize := (ext2fs.SuperBlock.S_inodes_count + 7) / 8 // Redondear hacia arriba
	bitmap := make([]byte, bitmapSize)

	_, err := file.ReadAt(bitmap, int64(ext2fs.SuperBlock.S_bm_inode_start))
	if err != nil {
		return -1, fmt.Errorf("error leyendo bitmap de inodos: %v", err)
	}

	// Buscar el primer bit libre (0)
	for i := int32(1); i < ext2fs.SuperBlock.S_inodes_count; i++ { // Empezar desde 1, no 0 (raíz)
		byteIndex := i / 8
		bitIndex := i % 8

		if byteIndex >= int32(len(bitmap)) {
			break
		}

		// Si el bit está en 0, está libre
		if (bitmap[byteIndex] & (1 << bitIndex)) == 0 {
			// Marcar como usado
			bitmap[byteIndex] |= (1 << bitIndex)

			// Escribir el bitmap actualizado
			_, err = file.WriteAt(bitmap, int64(ext2fs.SuperBlock.S_bm_inode_start))
			if err != nil {
				return -1, fmt.Errorf("error escribiendo bitmap de inodos: %v", err)
			}

			return i, nil
		}
	}

	return -1, fmt.Errorf("no hay inodos libres disponibles")
}

func findFreeInodeEXT3Move(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem) (int32, error) {
	// Implementación similar a findFreeInodeEXT2Move, adaptada para EXT3
	bitmapSize := (ext3fs.SuperBlock.S_inodes_count + 7) / 8
	bitmap := make([]byte, bitmapSize)

	_, err := file.ReadAt(bitmap, int64(ext3fs.SuperBlock.S_bm_inode_start))
	if err != nil {
		return -1, fmt.Errorf("error leyendo bitmap de inodos: %v", err)
	}

	for i := int32(1); i < ext3fs.SuperBlock.S_inodes_count; i++ {
		byteIndex := i / 8
		bitIndex := i % 8

		if byteIndex >= int32(len(bitmap)) {
			break
		}

		if (bitmap[byteIndex] & (1 << bitIndex)) == 0 {
			bitmap[byteIndex] |= (1 << bitIndex)

			_, err = file.WriteAt(bitmap, int64(ext3fs.SuperBlock.S_bm_inode_start))
			if err != nil {
				return -1, fmt.Errorf("error escribiendo bitmap de inodos: %v", err)
			}

			return i, nil
		}
	}

	return -1, fmt.Errorf("no hay inodos libres disponibles")
}
