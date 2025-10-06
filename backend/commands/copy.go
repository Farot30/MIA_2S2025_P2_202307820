package commandsP2

import (
	BloquesP2 "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt2P2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	sessionP2 "MIA_2S2025_P2_202307820/Session"
	EstructuraarchivocarpetaP2 "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func COPY(args []string) {
	if !sessionP2.IsSessionActive() {
		fmt.Printf("ERROR: No active session. Please log in first.\n")
		return
	}

	params := utilitiesP2.ParseParameters(args)
	sourcePath := strings.Trim(params["path"], "\"")
	destPath := strings.Trim(params["destino"], "\"")

	// Validar parámetros obligatorios
	if sourcePath == "" || destPath == "" {
		fmt.Println("ERROR: Faltan parámetros obligatorios para el comando copy: 'path' y 'destino'.")
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

	// Detectar tipo de sistema de archivos
	fsType, err := detectFileSystemType(mountedPartition)
	if err != nil {
		fmt.Printf("ERROR: No se pudo detectar el tipo de sistema de archivos: %v\n", err)
		return
	}

	fmt.Printf("=== COMANDO COPY ===\n")
	fmt.Printf("Sistema de archivos detectado: %s\n", getFileSystemTypeName(fsType))
	fmt.Printf("Origen: %s\n", sourcePath)
	fmt.Printf("Destino: %s\n", destPath)
	fmt.Printf("Usuario: %s\n", sessionInfo.Username)
	fmt.Printf("===================\n\n")

	// Ejecutar según el tipo de sistema de archivos
	if fsType == EstructuraarchivocarpetaP2.EXT3_FILESYSTEM_TYPE {
		err = copyInEXT3(mountedPartition, sourcePath, destPath, sessionInfo)
	} else {
		err = copyInEXT2(mountedPartition, sourcePath, destPath, sessionInfo)
	}

	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}

	fmt.Printf("Copia exitosa de '%s' a '%s'\n", sourcePath, destPath)
}

func detectFileSystemType(partition MountedPartition) (int32, error) {
	file, err := os.Open(partition.Path)
	if err != nil {
		return 0, fmt.Errorf("error abriendo la partición: %v", err)
	}
	defer file.Close()

	// Leer el superbloque
	superBlock := &EstructuraarchivocarpetaP2.SuperBloque{}
	err = utilitiesP2.ReadStruct(file, int64(partition.Start), superBlock)
	if err != nil {
		return 0, fmt.Errorf("error leyendo superbloque: %v", err)
	}

	return superBlock.S_filesystem_type, nil
}

// copyInEXT3 maneja la copia de archivos/carpetas en sistema EXT3 con journaling
func copyInEXT3(partition MountedPartition, sourcePath, destPath string, sessionInfo *sessionP2.UserSession) error {
	// Abrir archivo de la partición
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("no se pudo abrir la partición: %v", err)
	}
	defer file.Close()

	// Leer el sistema de archivos EXT3
	ext3fs, err := EstructuraExt3P2.ReadEXT3FileSystem(partition.Path, partition.Start)
	if err != nil {
		return fmt.Errorf("no se pudo leer el sistema EXT3: %v", err)
	}

	// REGISTRO EN JOURNAL: Inicio de operación de copia
	copyInfo := fmt.Sprintf("user=%s src=%s dst=%s", sessionInfo.Username, sourcePath, destPath)
	LogEXT3Operation(EstructuraExt3P2.OPERATION_RECOVERY, sourcePath, "copy_start: "+copyInfo)

	// Separar directorio padre y nombre del elemento fuente
	sourceDir := filepath.Dir(sourcePath)
	sourceName := filepath.Base(sourcePath)

	// Buscar el directorio padre del elemento fuente
	sourceDirInodeNum, err := findDirectoryInEXT3ForCopy(file, ext3fs, sourceDir)
	if err != nil {
		LogEXT3Operation(EstructuraExt3P2.OPERATION_RECOVERY, sourcePath, "copy_error: source directory not found")
		return fmt.Errorf("no se pudo encontrar el directorio padre fuente '%s': %v", sourceDir, err)
	}

	// Buscar el directorio de destino
	destDirInodeNum, err := findDirectoryInEXT3ForCopy(file, ext3fs, destPath)
	if err != nil {
		LogEXT3Operation(EstructuraExt3P2.OPERATION_RECOVERY, sourcePath, "copy_error: destination directory not found")
		return fmt.Errorf("no se pudo encontrar el directorio destino '%s': %v", destPath, err)
	}

	// Leer inodo del directorio padre fuente
	sourceDirInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+sourceDirInodeNum*ext3fs.SuperBlock.S_inode_s), sourceDirInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer inodo del directorio padre fuente: %v", err)
	}

	// Leer inodo del directorio destino
	destDirInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+destDirInodeNum*ext3fs.SuperBlock.S_inode_s), destDirInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer inodo del directorio destino: %v", err)
	}

	// Verificar permisos de escritura en el directorio destino
	canWrite, err := sessionP2.CheckPermissions(destDirInode, sessionP2.WRITE)
	if err != nil || !canWrite {
		LogEXT3Operation(EstructuraExt3P2.OPERATION_RECOVERY, destPath, "copy_error: no write permissions")
		return fmt.Errorf("no tiene permisos de escritura en el directorio destino '%s'", destPath)
	}

	// Buscar el elemento a copiar en el directorio fuente
	sourceInodeNum, err := findFileInDirectoryEXT3ForCopy(file, ext3fs, sourceDirInode, sourceName)
	if err != nil || sourceInodeNum == -1 {
		LogEXT3Operation(EstructuraExt3P2.OPERATION_RECOVERY, sourcePath, "copy_error: source file not found")
		return fmt.Errorf("no se pudo encontrar '%s' en '%s'", sourceName, sourceDir)
	}

	// Leer el inodo del elemento fuente
	sourceInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+sourceInodeNum*ext3fs.SuperBlock.S_inode_s), sourceInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer inodo del elemento fuente: %v", err)
	}

	// Verificar permisos de lectura en el elemento fuente
	canRead, err := sessionP2.CheckPermissions(sourceInode, sessionP2.READ)
	if err != nil || !canRead {
		LogEXT3Operation(EstructuraExt3P2.OPERATION_RECOVERY, sourcePath, "copy_error: no read permissions")
		return fmt.Errorf("no tiene permisos de lectura sobre el elemento '%s'", sourceName)
	}

	// Determinar estrategia según el tipo de elemento
	if sourceInode.Itype == 1 { // Archivo
		err = copyFileInEXT3Preserve(file, ext3fs, sourceInode, destDirInode, sourceName, sessionInfo)
	} else if sourceInode.Itype == 0 { // Directorio
		err = copyDirectoryInEXT3Preserve(file, ext3fs, sourceInode, destDirInode, sourceName, sessionInfo)
	} else {
		err = fmt.Errorf("tipo de inodo no reconocido: %d", sourceInode.Itype)
	}

	// REGISTRO EN JOURNAL: Resultado de la operación
	if err != nil {
		LogEXT3Operation(EstructuraExt3P2.OPERATION_RECOVERY, sourcePath, "copy_error: "+err.Error())
	} else {
		LogEXT3Operation(EstructuraExt3P2.OPERATION_RECOVERY, sourcePath, "copy_success: "+destPath)
	}

	return err
}

// copyFileInEXT3Preserve copia un archivo preservando todo su contenido en EXT3
func copyFileInEXT3Preserve(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, sourceInode *EstructuraarchivocarpetaP2.Inodo, destDirInode *EstructuraarchivocarpetaP2.Inodo, fileName string, sessionInfo *sessionP2.UserSession) error {
	fmt.Printf("[COPY-FILE-EXT3] Copiando archivo '%s' preservando contenido\n", fileName)

	// REGISTRO EN JOURNAL: Inicio de copia de archivo
	fileInfo := fmt.Sprintf("size=%d uid=%d gid=%d", sourceInode.Is, sourceInode.Iuid, sourceInode.Igid)
	LogEXT3Operation(EstructuraExt3P2.OPERATION_MKFILE, fileName, "file_copy_start: "+fileInfo)

	// 1. Leer todo el contenido del archivo fuente
	fileContent, err := ReadFileContentCorrectEXT3(file, ext3fs, sourceInode)
	if err != nil {
		fmt.Printf("[WARNING] No se pudo leer contenido de archivo '%s': %v\n", fileName, err)
		fileContent = []byte{} // Contenido vacío como fallback
		LogEXT3Operation(EstructuraExt3P2.OPERATION_MKFILE, fileName, "file_read_error: "+err.Error())
	}

	// 2. Crear estructura de contenido para preservación
	fileContentStruct := &DirectoryContent{
		Name:        fileName,
		IsDirectory: false,
		Permissions: sourceInode.Iperm,
		UID:         sourceInode.Iuid,
		GID:         sourceInode.Igid,
		Content:     fileContent,
	}

	// 3. Recrear el archivo en el destino
	err = createFileWithContentCopyEXT3(file, ext3fs, destDirInode, fileContentStruct)
	if err != nil {
		LogEXT3Operation(EstructuraExt3P2.OPERATION_MKFILE, fileName, "file_create_error: "+err.Error())
		return fmt.Errorf("no se pudo crear archivo en destino: %v", err)
	}

	// REGISTRO EN JOURNAL: Éxito en copia de archivo
	successInfo := fmt.Sprintf("copied %d bytes", len(fileContent))
	LogEXT3Operation(EstructuraExt3P2.OPERATION_MKFILE, fileName, "file_copy_success: "+successInfo)

	fmt.Printf("[COPY-FILE-EXT3] Archivo '%s' copiado exitosamente (%d bytes)\n", fileName, len(fileContent))
	return nil
}

// copyDirectoryInEXT3Preserve copia un directorio usando estrategia de preservación de contenido en EXT3
func copyDirectoryInEXT3Preserve(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, sourceInode *EstructuraarchivocarpetaP2.Inodo, destDirInode *EstructuraarchivocarpetaP2.Inodo, dirName string, sessionInfo *sessionP2.UserSession) error {
	fmt.Printf("[COPY-DIR-EXT3] Copiando directorio '%s' usando estrategia de preservación\n", dirName)

	// REGISTRO EN JOURNAL: Inicio de copia de directorio
	dirInfo := fmt.Sprintf("uid=%d gid=%d", sourceInode.Iuid, sourceInode.Igid)
	LogEXT3Operation(EstructuraExt3P2.OPERATION_MKDIR, dirName, "dir_copy_start: "+dirInfo)

	// 1. Guardar todo el contenido del directorio recursivamente
	dirContent, err := saveDirectoryContentCopyEXT3(file, ext3fs, sourceInode, dirName)
	if err != nil {
		LogEXT3Operation(EstructuraExt3P2.OPERATION_MKDIR, dirName, "dir_save_error: "+err.Error())
		return fmt.Errorf("no se pudo guardar contenido del directorio: %v", err)
	}

	// 2. Recrear el directorio en el destino (SIN eliminar el original)
	newDirInodeNum, err := createDirectoryWithContentCopyEXT3(file, ext3fs, destDirInode, dirContent, sessionInfo)
	if err != nil {
		LogEXT3Operation(EstructuraExt3P2.OPERATION_MKDIR, dirName, "dir_create_error: "+err.Error())
		return fmt.Errorf("no se pudo recrear directorio en destino: %v", err)
	}

	// REGISTRO EN JOURNAL: Éxito en copia de directorio
	successInfo := fmt.Sprintf("new_inode=%d children=%d", newDirInodeNum, len(dirContent.Children))
	LogEXT3Operation(EstructuraExt3P2.OPERATION_MKDIR, dirName, "dir_copy_success: "+successInfo)

	fmt.Printf("[COPY-DIR-EXT3] Directorio '%s' copiado exitosamente (nuevo inodo: %d)\n", dirName, newDirInodeNum)
	return nil
}

// saveDirectoryContentCopyEXT3 guarda recursivamente todo el contenido de un directorio (versión EXT3)
func saveDirectoryContentCopyEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, dirName string) (*DirectoryContent, error) {
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
					LogEXT3Operation(EstructuraExt3P2.OPERATION_RECOVERY, entryName, "inode_read_error: "+err.Error())
					continue
				}

				// Verificar permisos de lectura antes de copiar
				canRead, err := sessionP2.CheckPermissions(entryInode, sessionP2.READ)
				if err != nil || !canRead {
					fmt.Printf("[WARNING] Sin permisos de lectura para '%s', saltando...\n", entryName)
					LogEXT3Operation(EstructuraExt3P2.OPERATION_RECOVERY, entryName, "no_read_permissions")
					continue
				}

				if entryInode.Itype == 1 { // Archivo
					// Leer contenido del archivo
					fileContent, err := ReadFileContentCorrectEXT3(file, ext3fs, entryInode)
					if err != nil {
						fmt.Printf("[WARNING] No se pudo leer contenido de archivo '%s': %v\n", entryName, err)
						fileContent = []byte{} // Contenido vacío como fallback
						LogEXT3Operation(EstructuraExt3P2.OPERATION_RECOVERY, entryName, "file_content_read_error: "+err.Error())
					}

					dirContent.Children[entryName] = &DirectoryContent{
						Name:        entryName,
						IsDirectory: false,
						Permissions: entryInode.Iperm,
						UID:         entryInode.Iuid,
						GID:         entryInode.Igid,
						Content:     fileContent,
					}

					// Registro en journal de archivo guardado
					saveInfo := fmt.Sprintf("size=%d", len(fileContent))
					LogEXT3Operation(EstructuraExt3P2.OPERATION_RECOVERY, entryName, "file_saved: "+saveInfo)

					fmt.Printf("[SAVE-FILE-EXT3] Archivo '%s' guardado (%d bytes)\n", entryName, len(fileContent))

				} else if entryInode.Itype == 0 { // Directorio
					// Guardar recursivamente
					subDirContent, err := saveDirectoryContentCopyEXT3(file, ext3fs, entryInode, entryName)
					if err != nil {
						fmt.Printf("[WARNING] No se pudo guardar subdirectorio '%s': %v\n", entryName, err)
						LogEXT3Operation(EstructuraExt3P2.OPERATION_RECOVERY, entryName, "subdir_save_error: "+err.Error())
						continue
					}

					dirContent.Children[entryName] = subDirContent

					// Registro en journal de directorio guardado
					LogEXT3Operation(EstructuraExt3P2.OPERATION_RECOVERY, entryName, "dir_saved_recursive")

					fmt.Printf("[SAVE-DIR-EXT3] Directorio '%s' guardado recursivamente\n", entryName)
				}
			}
		}
	}

	fmt.Printf("[SAVE-CONTENT-EXT3] Contenido del directorio '%s' guardado (%d elementos)\n", dirName, len(dirContent.Children))
	return dirContent, nil
}

// createDirectoryWithContentCopyEXT3 recrea un directorio con todo su contenido (versión EXT3)
func createDirectoryWithContentCopyEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, parentInode *EstructuraarchivocarpetaP2.Inodo, dirContent *DirectoryContent, sessionInfo *sessionP2.UserSession) (int32, error) {
	fmt.Printf("[CREATE-CONTENT-EXT3] Recreando directorio '%s' con contenido\n", dirContent.Name)

	// Crear el directorio base
	newDirInodeNum, err := createBasicDirectoryCopyEXT3(file, ext3fs, parentInode, dirContent.Name, dirContent.UID, dirContent.GID, dirContent.Permissions)
	if err != nil {
		return -1, fmt.Errorf("no se pudo crear directorio base: %v", err)
	}

	// Leer el inodo del nuevo directorio
	newDirInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+newDirInodeNum*ext3fs.SuperBlock.S_inode_s), newDirInode)
	if err != nil {
		return -1, fmt.Errorf("no se pudo leer inodo del nuevo directorio: %v", err)
	}

	// Recrear todo el contenido
	for name, content := range dirContent.Children {
		if content.IsDirectory {
			// Recrear subdirectorio recursivamente
			_, err := createDirectoryWithContentCopyEXT3(file, ext3fs, newDirInode, content, sessionInfo)
			if err != nil {
				fmt.Printf("[WARNING] No se pudo recrear subdirectorio '%s': %v\n", name, err)
				LogEXT3Operation(EstructuraExt3P2.OPERATION_MKDIR, name, "subdir_recreate_error: "+err.Error())
			} else {
				LogEXT3Operation(EstructuraExt3P2.OPERATION_MKDIR, name, "subdir_recreated")
			}
		} else {
			// Recrear archivo
			err := createFileWithContentCopyEXT3(file, ext3fs, newDirInode, content)
			if err != nil {
				fmt.Printf("[WARNING] No se pudo recrear archivo '%s': %v\n", name, err)
				LogEXT3Operation(EstructuraExt3P2.OPERATION_MKFILE, name, "file_recreate_error: "+err.Error())
			} else {
				recreateInfo := fmt.Sprintf("size=%d", len(content.Content))
				LogEXT3Operation(EstructuraExt3P2.OPERATION_MKFILE, name, "file_recreated: "+recreateInfo)
			}
		}
	}

	fmt.Printf("[CREATE-CONTENT-EXT3] Directorio '%s' recreado exitosamente\n", dirContent.Name)
	return newDirInodeNum, nil
}

// createBasicDirectoryCopyEXT3 crea un directorio básico con propiedades específicas (versión EXT3)
func createBasicDirectoryCopyEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, parentInode *EstructuraarchivocarpetaP2.Inodo, dirName string, uid, gid int32, permissions [3]byte) (int32, error) {
	// Buscar un inodo libre
	newInodeNum, err := EstructuraExt3P2.FindFreeInodeEXT3(file, ext3fs)
	if err != nil {
		return -1, fmt.Errorf("no se pudo encontrar un inodo libre: %v", err)
	}

	// Buscar un bloque libre para el directorio
	newBlockNum, err := findFreeBlockEXT3(file, ext3fs)
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
	dirBlock.AddEntry(".", newInodeNum)
	dirBlock.AddEntry("..", getParentInodeNumCopyEXT3(file, ext3fs, parentInode))

	// Escribir el bloque del directorio
	err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+newBlockNum*ext3fs.SuperBlock.S_block_s), dirBlock)
	if err != nil {
		return -1, fmt.Errorf("error escribiendo bloque del directorio: %v", err)
	}

	// Escribir el inodo del directorio
	err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_inode_start+newInodeNum*ext3fs.SuperBlock.S_inode_s), newDirInode)
	if err != nil {
		return -1, fmt.Errorf("error escribiendo inodo del directorio: %v", err)
	}

	// Agregar entrada al directorio padre
	err = addEntryToDirectoryCopyEXT3(file, ext3fs, parentInode, dirName, newInodeNum)
	if err != nil {
		return -1, fmt.Errorf("error agregando entrada al directorio padre: %v", err)
	}

	return newInodeNum, nil
}

// createFileWithContentCopyEXT3 crea un archivo con contenido específico (versión EXT3)
func createFileWithContentCopyEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, parentInode *EstructuraarchivocarpetaP2.Inodo, fileContent *DirectoryContent) error {
	// Buscar un inodo libre
	newInodeNum, err := EstructuraExt3P2.FindFreeInodeEXT3(file, ext3fs)
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

	// Escribir el contenido usando la función adaptada para EXT3
	err = WriteFileContentCorrectEXT3(file, ext3fs, newFileInode, fileContent.Content)
	if err != nil {
		return fmt.Errorf("error escribiendo contenido del archivo: %v", err)
	}

	// Escribir el inodo del archivo
	err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_inode_start+newInodeNum*ext3fs.SuperBlock.S_inode_s), newFileInode)
	if err != nil {
		return fmt.Errorf("error escribiendo inodo del archivo: %v", err)
	}

	// Agregar entrada al directorio padre
	err = addEntryToDirectoryCopyEXT3(file, ext3fs, parentInode, fileContent.Name, newInodeNum)
	if err != nil {
		return fmt.Errorf("error agregando entrada al directorio padre: %v", err)
	}

	return nil
}

// === FUNCIONES AUXILIARES PARA EXT3 ===

// ReadFileContentCorrectEXT3 lee correctamente todo el contenido de un archivo en EXT3
func ReadFileContentCorrectEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, inode *EstructuraarchivocarpetaP2.Inodo) ([]byte, error) {
	if inode.Itype != 1 {
		return nil, fmt.Errorf("el inodo no es un archivo")
	}

	totalSize := inode.Is
	if totalSize <= 0 {
		return []byte{}, nil
	}

	var content []byte
	bytesRead := int32(0)

	// Leer bloques directos
	for i := 0; i < 12 && inode.Iblock[i] != -1 && bytesRead < totalSize; i++ {
		blockNum := inode.Iblock[i]
		block := &BloquesP2.BloqueArchivo{}

		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+blockNum*ext3fs.SuperBlock.S_block_s), block)
		if err != nil {
			return nil, fmt.Errorf("error leyendo bloque %d: %v", blockNum, err)
		}

		bytesToRead := min(64, int(totalSize-bytesRead))
		blockContent := GetBytesContent(block, bytesToRead)
		content = append(content, blockContent...)
		bytesRead += int32(bytesToRead)
	}

	// Bloques indirectos si es necesario
	if inode.Iblock[12] != -1 && bytesRead < totalSize {
		// Implementar lectura de bloques indirectos para EXT3
		// Similar a EXT2 pero usando estructuras EXT3
	}

	return content[:min(len(content), int(totalSize))], nil
}

// WriteFileContentCorrectEXT3 escribe el contenido a los bloques del archivo en EXT3
func WriteFileContentCorrectEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, inode *EstructuraarchivocarpetaP2.Inodo, content []byte) error {
	contentSize := len(content)
	if contentSize == 0 {
		return nil // No hay contenido que escribir
	}

	bytesWritten := 0
	blockIndex := 0

	// Escribir bloques directos
	for blockIndex < 12 && bytesWritten < contentSize {
		// Buscar un bloque libre
		blockNum, err := findFreeBlockEXT3(file, ext3fs)
		if err != nil || blockNum == -1 {
			return fmt.Errorf("no se pudo encontrar un bloque libre: %v", err)
		}

		// Calcular cuántos bytes escribir en este bloque
		bytesToWrite := min(64, contentSize-bytesWritten)

		// Crear el bloque de archivo
		block := &BloquesP2.BloqueArchivo{}
		blockContent := content[bytesWritten : bytesWritten+bytesToWrite]
		SetBytesContent(block, blockContent)

		// Escribir el bloque al disco
		err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+blockNum*ext3fs.SuperBlock.S_block_s), block)
		if err != nil {
			return fmt.Errorf("error escribiendo bloque %d: %v", blockNum, err)
		}

		// Asignar el bloque al inodo
		inode.Iblock[blockIndex] = blockNum
		blockIndex++
		bytesWritten += bytesToWrite
	}

	// Si necesitamos más bloques, usar bloques indirectos
	if bytesWritten < contentSize {
		// Implementar escritura de bloques indirectos para EXT3
		// Similar a EXT2 pero usando estructuras EXT3
	}

	return nil
}

// getParentInodeNumCopyEXT3 obtiene el número de inodo del directorio padre (versión EXT3)
func getParentInodeNumCopyEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, parentInode *EstructuraarchivocarpetaP2.Inodo) int32 {
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
	return 0 // Por defecto, el directorio raíz
}

// addEntryToDirectoryCopyEXT3 agrega una entrada a un directorio (versión EXT3)
func addEntryToDirectoryCopyEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, entryName string, entryInodeNum int32) error {
	// Buscar espacio libre en los bloques existentes
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

	// Si no hay espacio, crear un nuevo bloque
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

	// Asignar el bloque al directorio
	for i := 0; i < 12; i++ {
		if dirInode.Iblock[i] == -1 {
			dirInode.Iblock[i] = newBlockNum
			break
		}
	}

	return nil
}

// copyInEXT2 maneja la copia de archivos/carpetas en sistema EXT2
func copyInEXT2(partition MountedPartition, sourcePath, destPath string, sessionInfo *sessionP2.UserSession) error {
	// Abrir archivo de la partición
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("no se pudo abrir la partición: %v", err)
	}
	defer file.Close()

	// Leer el sistema de archivos EXT2
	ext2fs, err := EstructuraExt2P2.ReadEXT2FileSystem(partition.Path, partition.Start)
	if err != nil {
		return fmt.Errorf("no se pudo leer el sistema EXT2: %v", err)
	}

	// Separar directorio padre y nombre del elemento fuente
	sourceDir := filepath.Dir(sourcePath)
	sourceName := filepath.Base(sourcePath)

	// Buscar el directorio padre del elemento fuente
	sourceDirInodeNum, err := findDirectoryInEXT2ForCopy(file, ext2fs, sourceDir)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar el directorio padre fuente '%s': %v", sourceDir, err)
	}

	// Buscar el directorio de destino
	destDirInodeNum, err := findDirectoryInEXT2ForCopy(file, ext2fs, destPath)
	if err != nil {
		return fmt.Errorf("no se pudo encontrar el directorio destino '%s': %v", destPath, err)
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
		return fmt.Errorf("no tiene permisos de escritura en el directorio destino '%s'", destPath)
	}

	// Buscar el elemento a copiar en el directorio fuente
	sourceInodeNum, err := findFileInDirectoryEXT2ForCopy(file, ext2fs, sourceDirInode, sourceName)
	if err != nil || sourceInodeNum == -1 {
		return fmt.Errorf("no se pudo encontrar '%s' en '%s'", sourceName, sourceDir)
	}

	// Leer el inodo del elemento fuente
	sourceInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+sourceInodeNum*ext2fs.SuperBlock.S_inode_s), sourceInode)
	if err != nil {
		return fmt.Errorf("no se pudo leer inodo del elemento fuente: %v", err)
	}

	// Verificar permisos de lectura en el elemento fuente
	canRead, err := sessionP2.CheckPermissions(sourceInode, sessionP2.READ)
	if err != nil || !canRead {
		return fmt.Errorf("no tiene permisos de lectura sobre el elemento '%s'", sourceName)
	}

	// Determinar estrategia según el tipo de elemento
	if sourceInode.Itype == 1 { // Archivo
		return copyFileInEXT2Preserve(file, ext2fs, sourceInode, destDirInode, sourceName, sessionInfo)
	} else if sourceInode.Itype == 0 { // Directorio
		return copyDirectoryInEXT2Preserve(file, ext2fs, sourceInode, destDirInode, sourceName, sessionInfo)
	}

	return fmt.Errorf("tipo de inodo no reconocido: %d", sourceInode.Itype)
}

// copyFileInEXT2Preserve copia un archivo preservando todo su contenido
func copyFileInEXT2Preserve(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, sourceInode *EstructuraarchivocarpetaP2.Inodo, destDirInode *EstructuraarchivocarpetaP2.Inodo, fileName string, sessionInfo *sessionP2.UserSession) error {
	fmt.Printf("[COPY-FILE] Copiando archivo '%s' preservando contenido\n", fileName)

	// 1. Leer todo el contenido del archivo fuente
	fileContent, err := ReadFileContentCorrect(file, ext2fs, sourceInode)
	if err != nil {
		fmt.Printf("[WARNING] No se pudo leer contenido de archivo '%s': %v\n", fileName, err)
		fileContent = []byte{} // Contenido vacío como fallback
	}

	// 2. Crear estructura de contenido para preservación
	fileContentStruct := &DirectoryContent{
		Name:        fileName,
		IsDirectory: false,
		Permissions: sourceInode.Iperm,
		UID:         sourceInode.Iuid,
		GID:         sourceInode.Igid,
		Content:     fileContent,
	}

	// 3. Recrear el archivo en el destino
	err = createFileWithContentCopy(file, ext2fs, destDirInode, fileContentStruct)
	if err != nil {
		return fmt.Errorf("no se pudo crear archivo en destino: %v", err)
	}

	fmt.Printf("[COPY-FILE] Archivo '%s' copiado exitosamente (%d bytes)\n", fileName, len(fileContent))
	return nil
}

// copyDirectoryInEXT2Preserve copia un directorio usando estrategia de preservación de contenido
func copyDirectoryInEXT2Preserve(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, sourceInode *EstructuraarchivocarpetaP2.Inodo, destDirInode *EstructuraarchivocarpetaP2.Inodo, dirName string, sessionInfo *sessionP2.UserSession) error {
	fmt.Printf("[COPY-DIR] Copiando directorio '%s' usando estrategia de preservación\n", dirName)

	// 1. Guardar todo el contenido del directorio recursivamente
	dirContent, err := saveDirectoryContentCopy(file, ext2fs, sourceInode, dirName)
	if err != nil {
		return fmt.Errorf("no se pudo guardar contenido del directorio: %v", err)
	}

	// 2. Recrear el directorio en el destino (SIN eliminar el original)
	newDirInodeNum, err := createDirectoryWithContentCopy(file, ext2fs, destDirInode, dirContent, sessionInfo)
	if err != nil {
		return fmt.Errorf("no se pudo recrear directorio en destino: %v", err)
	}

	fmt.Printf("[COPY-DIR] Directorio '%s' copiado exitosamente (nuevo inodo: %d)\n", dirName, newDirInodeNum)
	return nil
}

// saveDirectoryContentCopy guarda recursivamente todo el contenido de un directorio (versión para COPY)
func saveDirectoryContentCopy(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, dirName string) (*DirectoryContent, error) {
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

				// Verificar permisos de lectura antes de copiar
				canRead, err := sessionP2.CheckPermissions(entryInode, sessionP2.READ)
				if err != nil || !canRead {
					fmt.Printf("[WARNING] Sin permisos de lectura para '%s', saltando...\n", entryName)
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
					subDirContent, err := saveDirectoryContentCopy(file, ext2fs, entryInode, entryName)
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

// createDirectoryWithContentCopy recrea un directorio con todo su contenido (versión para COPY)
func createDirectoryWithContentCopy(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, parentInode *EstructuraarchivocarpetaP2.Inodo, dirContent *DirectoryContent, sessionInfo *sessionP2.UserSession) (int32, error) {
	fmt.Printf("[CREATE-CONTENT] Recreando directorio '%s' con contenido\n", dirContent.Name)

	// Crear el directorio base
	newDirInodeNum, err := createBasicDirectoryCopy(file, ext2fs, parentInode, dirContent.Name, dirContent.UID, dirContent.GID, dirContent.Permissions)
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
			_, err := createDirectoryWithContentCopy(file, ext2fs, newDirInode, content, sessionInfo)
			if err != nil {
				fmt.Printf("[WARNING] No se pudo recrear subdirectorio '%s': %v\n", name, err)
			}
		} else {
			// Recrear archivo
			err := createFileWithContentCopy(file, ext2fs, newDirInode, content)
			if err != nil {
				fmt.Printf("[WARNING] No se pudo recrear archivo '%s': %v\n", name, err)
			}
		}
	}

	fmt.Printf("[CREATE-CONTENT] Directorio '%s' recreado exitosamente\n", dirContent.Name)
	return newDirInodeNum, nil
}

// createBasicDirectoryCopy crea un directorio básico con propiedades específicas (versión para COPY)
func createBasicDirectoryCopy(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, parentInode *EstructuraarchivocarpetaP2.Inodo, dirName string, uid, gid int32, permissions [3]byte) (int32, error) {
	// Buscar un inodo libre
	newInodeNum, err := findFreeInodeEXT2Copy(file, ext2fs)
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
	dirBlock.AddEntry(".", newInodeNum)                                       // Entrada actual
	dirBlock.AddEntry("..", getParentInodeNumCopy(file, ext2fs, parentInode)) // Entrada padre

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
	err = addEntryToDirectoryCopy(file, ext2fs, parentInode, dirName, newInodeNum)
	if err != nil {
		return -1, fmt.Errorf("error agregando entrada al directorio padre: %v", err)
	}

	return newInodeNum, nil
}

// createFileWithContentCopy crea un archivo con contenido específico (versión para COPY)
func createFileWithContentCopy(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, parentInode *EstructuraarchivocarpetaP2.Inodo, fileContent *DirectoryContent) error {
	// Buscar un inodo libre
	newInodeNum, err := findFreeInodeEXT2Copy(file, ext2fs)
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
	err = addEntryToDirectoryCopy(file, ext2fs, parentInode, fileContent.Name, newInodeNum)
	if err != nil {
		return fmt.Errorf("error agregando entrada al directorio padre: %v", err)
	}

	return nil
}

// Funciones auxiliares específicas para COPY (para evitar conflictos de nombres)

// getParentInodeNumCopy obtiene el número de inodo del directorio padre (versión para COPY)
func getParentInodeNumCopy(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, parentInode *EstructuraarchivocarpetaP2.Inodo) int32 {
	if parentInode.Iblock[0] != -1 {
		block := &BloquesP2.BloqueCarpeta{}
		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+parentInode.Iblock[0]*ext2fs.SuperBlock.S_block_s), block)
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
	return 0 // Por defecto, el directorio raíz
}

// findFreeInodeEXT2Copy busca un inodo libre en el sistema EXT2 (versión para COPY)
func findFreeInodeEXT2Copy(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem) (int32, error) {
	bitmapSize := (ext2fs.SuperBlock.S_inodes_count + 7) / 8
	bitmap := make([]byte, bitmapSize)

	_, err := file.ReadAt(bitmap, int64(ext2fs.SuperBlock.S_bm_inode_start))
	if err != nil {
		return -1, fmt.Errorf("error leyendo bitmap de inodos: %v", err)
	}

	for i := int32(1); i < ext2fs.SuperBlock.S_inodes_count; i++ {
		byteIndex := i / 8
		bitIndex := i % 8

		if byteIndex >= int32(len(bitmap)) {
			break
		}

		if (bitmap[byteIndex] & (1 << bitIndex)) == 0 {
			bitmap[byteIndex] |= (1 << bitIndex)
			_, err = file.WriteAt(bitmap, int64(ext2fs.SuperBlock.S_bm_inode_start))
			if err != nil {
				return -1, fmt.Errorf("error escribiendo bitmap de inodos: %v", err)
			}
			return i, nil
		}
	}

	return -1, fmt.Errorf("no hay inodos libres disponibles")
}

// addEntryToDirectoryCopy agrega una entrada a un directorio (versión para COPY)
func addEntryToDirectoryCopy(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, entryName string, entryInodeNum int32) error {
	// Buscar espacio libre en los bloques existentes
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		block := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+blockNum*ext2fs.SuperBlock.S_block_s), block)
		if err != nil {
			continue
		}

		if block.AddEntry(entryName, entryInodeNum) {
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

	newBlock := BloquesP2.NewBloqueCarpeta()
	newBlock.AddEntry(entryName, entryInodeNum)

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
