package commandsP2

import (
	BloquesP2 "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt3 "MIA_2S2025_P2_202307820/EstructuraExt3"
	sessionP2 "MIA_2S2025_P2_202307820/Session"
	estructuraArchivoCarpeta "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

type FileSystemItem struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	IsDirectory bool   `json:"is_directory"`
	Size        int64  `json:"size"`
	Permissions string `json:"permissions"`
	Owner       string `json:"owner"`
	Group       string `json:"group"`
	Modified    string `json:"modified"`
	Content     string `json:"content,omitempty"`
}

type FileSystemContentResponse struct {
	Success       bool             `json:"success"`
	CurrentPath   string           `json:"current_path"`
	ParentPath    string           `json:"parent_path"`
	Items         []FileSystemItem `json:"items"`
	PartitionInfo PartitionInfo    `json:"partition_info"`
	Message       string           `json:"message"`
	Error         string           `json:"error,omitempty"`
}

// HandleFileSystemContentAPI maneja las peticiones para obtener contenido del sistema de archivos
func HandleFileSystemContentAPI(w http.ResponseWriter, r *http.Request) {
	// Permitir CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		return
	}

	if r.Method != "GET" {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	// Obtener parámetros
	partitionID := r.URL.Query().Get("partition_id")
	path := r.URL.Query().Get("path")
	showContent := r.URL.Query().Get("show_content") == "true"

	if partitionID == "" {
		response := FileSystemContentResponse{
			Success: false,
			Error:   "Parámetro 'partition_id' es requerido",
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	if path == "" {
		path = "/"
	}

	// Verificar que hay una sesión activa
	if !sessionP2.IsSessionActive() {
		response := FileSystemContentResponse{
			Success: false,
			Error:   "No hay sesión activa. Debe iniciar sesión primero.",
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	session := sessionP2.GetCurrentSession()
	if session.PartitionID != partitionID {
		response := FileSystemContentResponse{
			Success: false,
			Error:   "La sesión actual no corresponde a la partición solicitada",
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Obtener contenido del directorio
	items, err := getDirectoryContentDirect(partitionID, path, showContent)
	if err != nil {
		response := FileSystemContentResponse{
			Success: false,
			Error:   fmt.Sprintf("Error obteniendo contenido: %s", err.Error()),
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Calcular ruta padre
	parentPath := "/"
	if path != "/" {
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) > 1 {
			parentPath = "/" + strings.Join(parts[:len(parts)-1], "/")
		}
	}

	// Obtener información de la partición
	mountedPartition, exists := GetMountedPartition(partitionID)
	if !exists {
		response := FileSystemContentResponse{
			Success: false,
			Error:   "Partición no montada",
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	partitionInfo := PartitionInfo{
		Name:       mountedPartition.Name,
		Type:       mountedPartition.Type,
		IsMounted:  true,
		MountedID:  partitionID,
		FileSystem: mountedPartition.Type,
	}

	response := FileSystemContentResponse{
		Success:       true,
		CurrentPath:   path,
		ParentPath:    parentPath,
		Items:         items,
		PartitionInfo: partitionInfo,
		Message:       fmt.Sprintf("Contenido cargado exitosamente (%d elementos)", len(items)),
	}

	json.NewEncoder(w).Encode(response)
}

// getDirectoryContentDirect obtiene el contenido de un directorio leyendo directamente del sistema de archivos
func getDirectoryContentDirect(partitionID, path string, showContent bool) ([]FileSystemItem, error) {
	// Obtener la partición montada
	mountedPartition, exists := GetMountedPartition(partitionID)
	if !exists {
		return nil, fmt.Errorf("partición no montada")
	}

	// Detectar tipo de sistema de archivos
	fsType, err := DetectFileSystemType(mountedPartition)
	if err != nil {
		return nil, fmt.Errorf("error detectando sistema de archivos: %v", err)
	}

	// Leer contenido según el tipo de sistema de archivos
	switch fsType {
	case estructuraArchivoCarpeta.EXT3_FILESYSTEM_TYPE:
		return readDirectoryContentEXT3(mountedPartition, path, showContent)
	case estructuraArchivoCarpeta.EXT2_FILESYSTEM_TYPE:
		return readDirectoryContentEXT2(mountedPartition, path, showContent)
	default:
		return nil, fmt.Errorf("sistema de archivos no soportado: %d", fsType)
	}
}

// readDirectoryContentEXT3 lee el contenido de un directorio en EXT3
func readDirectoryContentEXT3(partition MountedPartition, path string, showContent bool) ([]FileSystemItem, error) {
	// Leer el sistema EXT3
	ext3fs, err := EstructuraExt3.ReadEXT3FileSystem(partition.Path, partition.Start)
	if err != nil {
		return nil, fmt.Errorf("error leyendo sistema EXT3: %v", err)
	}

	// Verificar magic number
	if ext3fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return nil, fmt.Errorf("sistema EXT3 no válido")
	}

	// Por ahora, usar EXT3 para listar archivos - esto depende de tu implementación de EXT3
	// Si tu EXT3 tiene un método para listar directorios, úsalo aquí
	return readDirectoryFromInodeEXT3(ext3fs, path, showContent)
}

// readDirectoryContentEXT2 lee el contenido de un directorio en EXT2
func readDirectoryContentEXT2(partition MountedPartition, path string, showContent bool) ([]FileSystemItem, error) {
	// Abrir el archivo del disco
	file, err := os.OpenFile(partition.Path, os.O_RDONLY, 0666)
	if err != nil {
		return nil, fmt.Errorf("error abriendo disco: %v", err)
	}
	defer file.Close()

	// Leer el sistema EXT2
	ext2fs, err := EstructuraExt2.ReadEXT2FileSystem(partition.Path, partition.Start)
	if err != nil {
		return nil, fmt.Errorf("error leyendo sistema EXT2: %v", err)
	}

	// Verificar magic number
	if ext2fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return nil, fmt.Errorf("sistema EXT2 no válido")
	}

	// Buscar el inodo del directorio
	dirInodeNum, err := findDirectoryByPaths(file, ext2fs, path)
	if err != nil {
		return nil, fmt.Errorf("directorio no encontrado: %v", err)
	}

	// Leer el contenido del directorio
	return readDirectoryFromInode(file, ext2fs, dirInodeNum, path, showContent)
}

// findDirectoryByPath busca un directorio por su ruta
func findDirectoryByPaths(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, dirPath string) (int32, error) {
	if dirPath == "/" {
		return 0, nil // Inodo raíz
	}

	// Usar la función existente findFileByPath
	return findFileByPath(file, ext2fs, dirPath)
}

// readDirectoryFromInode lee el contenido de un directorio desde su inodo
func readDirectoryFromInode(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, dirInodeNum int32, dirPath string, showContent bool) ([]FileSystemItem, error) {
	// Leer el inodo del directorio
	dirInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (dirInodeNum * ext2fs.SuperBlock.S_inode_s))
	err := utilitiesP2.ReadStruct(file, inodeOffset, dirInode)
	if err != nil {
		return nil, fmt.Errorf("error leyendo inodo del directorio: %v", err)
	}

	// Verificar que es un directorio
	if !dirInode.IsDirectory() {
		return nil, fmt.Errorf("el inodo %d no es un directorio", dirInodeNum)
	}

	var items []FileSystemItem

	// Leer los bloques del directorio
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]

		// Leer el bloque de carpeta
		dirBlock := &BloquesP2.BloqueCarpeta{}
		blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
		err = utilitiesP2.ReadStruct(file, blockOffset, dirBlock)
		if err != nil {
			continue
		}

		// Procesar las entradas del bloque
		for j := 0; j < 4; j++ {
			entry := dirBlock.B_content[j]
			if entry.B_inodo == -1 {
				continue
			}

			entryName := strings.TrimRight(string(entry.B_name[:]), "\x00")

			// Saltar . y ..
			if entryName == "." || entryName == ".." {
				continue
			}

			// Leer el inodo de la entrada
			entryInode := &estructuraArchivoCarpeta.Inodo{}
			entryInodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (entry.B_inodo * ext2fs.SuperBlock.S_inode_s))
			err = utilitiesP2.ReadStruct(file, entryInodeOffset, entryInode)
			if err != nil {
				continue
			}

			// Crear el item del sistema de archivos
			item := createFileSystemItem(entryName, dirPath, entryInode, showContent)

			items = append(items, item)
		}
	}

	return items, nil
}

// readDirectoryFromInodeEXT3 lee el contenido de un directorio en EXT3
func readDirectoryFromInodeEXT3(ext3fs *EstructuraExt3.EXT3FileSystem, path string, showContent bool) ([]FileSystemItem, error) {
	// Abrir el archivo del disco
	file, err := os.OpenFile(ext3fs.PartitionPath, os.O_RDONLY, 0666)
	if err != nil {
		return nil, fmt.Errorf("error abriendo disco: %v", err)
	}
	defer file.Close()

	// Encontrar el inodo del directorio por su ruta
	dirInodeNum, err := findDirectoryByPathEXT3(file, ext3fs, path)
	if err != nil {
		return nil, fmt.Errorf("directorio no encontrado: %v", err)
	}

	// Leer el inodo del directorio
	dirInode, err := ext3fs.LeerInodo(dirInodeNum)
	if err != nil {
		return nil, fmt.Errorf("error leyendo inodo del directorio: %v", err)
	}

	// Verificar que es un directorio
	if !dirInode.IsDirectory() {
		return nil, fmt.Errorf("el inodo %d no es un directorio", dirInodeNum)
	}

	var items []FileSystemItem

	// Función auxiliar para leer un bloque
	readBlock := func(blockNum int32) (*BloquesP2.BloqueCarpeta, error) {
		if blockNum < 0 || blockNum >= ext3fs.SuperBlock.S_blocks_count {
			return nil, fmt.Errorf("número de bloque fuera de rango: %d", blockNum)
		}

		dirBlock := &BloquesP2.BloqueCarpeta{}
		blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))
		err := utilitiesP2.ReadStruct(file, blockOffset, dirBlock)
		if err != nil {
			return nil, fmt.Errorf("error leyendo bloque %d: %v", blockNum, err)
		}
		return dirBlock, nil
	}

	// Procesar bloques directos (0-11)
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]

		dirBlock, err := readBlock(blockNum)
		if err != nil {
			continue // Saltar bloques con errores
		}

		// Procesar las entradas del bloque
		for j := 0; j < 4; j++ {
			entry := dirBlock.B_content[j]
			if entry.B_inodo == -1 {
				continue // Entrada vacía
			}

			entryName := strings.TrimRight(string(entry.B_name[:]), "\x00")

			// Saltar . y ..
			if entryName == "." || entryName == ".." {
				continue
			}

			// Leer el inodo de la entrada
			entryInode, err := ext3fs.LeerInodo(entry.B_inodo)
			if err != nil {
				continue // Saltar entradas con errores
			}

			// Crear el item del sistema de archivos
			item := createFileSystemItem(entryName, path, entryInode, showContent)

			items = append(items, item)
		}
	}

	// Procesar bloque indirecto simple (índice 12) si existe
	if dirInode.Iblock[12] != -1 {
		indirectBlockNum := dirInode.Iblock[12]

		// Leer el bloque de apuntadores
		indirectBlock := &BloquesP2.BloqueApuntador{}
		indirectBlockOffset := int64(ext3fs.SuperBlock.S_block_start + (indirectBlockNum * ext3fs.SuperBlock.S_block_s))
		err := utilitiesP2.ReadStruct(file, indirectBlockOffset, indirectBlock)
		if err == nil {
			// Procesar cada bloque apuntado
			for _, blockNum := range indirectBlock.B_pointers {
				if blockNum == -1 {
					continue
				}

				dirBlock, err := readBlock(blockNum)
				if err != nil {
					continue
				}

				// Procesar las entradas del bloque
				for j := 0; j < 4; j++ {
					entry := dirBlock.B_content[j]
					if entry.B_inodo == -1 {
						continue
					}

					entryName := strings.TrimRight(string(entry.B_name[:]), "\x00")

					// Saltar . y ..
					if entryName == "." || entryName == ".." {
						continue
					}

					// Leer el inodo de la entrada
					entryInode, err := ext3fs.LeerInodo(entry.B_inodo)
					if err != nil {
						continue
					}

					// Crear el item del sistema de archivos
					item := createFileSystemItem(entryName, path, entryInode, showContent)

					// Si es un archivo y se solicita contenido, obtenerlo
					if !item.IsDirectory && showContent && item.Size > 0 && item.Size < 1024*10 {
						var filePath string
						if path == "/" {
							filePath = "/" + entryName
						} else {
							filePath = path + "/" + entryName
						}

						content, err := ext3fs.LeerArchivo(filePath)
						if err == nil {
							item.Content = content
						}
					}

					items = append(items, item)
				}
			}
		}
	}

	// Procesar bloques indirectos dobles y triples si es necesario
	// (Por simplicidad, no los implemento aquí, pero seguirían el mismo patrón)

	return items, nil
}

// findDirectoryByPathEXT3 busca un directorio por su ruta en EXT3
func findDirectoryByPathEXT3(file *os.File, ext3fs *EstructuraExt3.EXT3FileSystem, dirPath string) (int32, error) {
	if dirPath == "/" {
		return 0, nil // Inodo raíz
	}

	// Dividir la ruta en componentes
	path := strings.Trim(dirPath, "/")
	if path == "" {
		return 0, nil // Ruta raíz
	}

	parts := strings.Split(path, "/")
	currentInodeNum := int32(0) // Empezar desde la raíz

	// Navegar por cada componente de la ruta
	for _, component := range parts {
		if component == "" {
			continue
		}

		// Leer el inodo actual (debería ser un directorio)
		currentInode, err := ext3fs.LeerInodo(currentInodeNum)
		if err != nil {
			return -1, fmt.Errorf("error leyendo inodo %d: %v", currentInodeNum, err)
		}

		if !currentInode.IsDirectory() {
			return -1, fmt.Errorf("el componente '%s' no es un directorio", component)
		}

		// Buscar el componente en el directorio actual
		found := false

		// Buscar en bloques directos
		for i := 0; i < 12 && currentInode.Iblock[i] != -1 && !found; i++ {
			blockNum := currentInode.Iblock[i]

			dirBlock := &BloquesP2.BloqueCarpeta{}
			blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))
			err := utilitiesP2.ReadStruct(file, blockOffset, dirBlock)
			if err != nil {
				continue
			}

			// Buscar en las entradas del bloque
			for j := 0; j < 4; j++ {
				entry := dirBlock.B_content[j]
				if entry.B_inodo == -1 {
					continue
				}

				entryName := strings.TrimRight(string(entry.B_name[:]), "\x00")
				if entryName == component {
					currentInodeNum = entry.B_inodo
					found = true
					break
				}
			}
		}

		// Si no se encontró en bloques directos, buscar en bloque indirecto
		if !found && currentInode.Iblock[12] != -1 {
			indirectBlockNum := currentInode.Iblock[12]

			indirectBlock := &BloquesP2.BloqueApuntador{}
			indirectBlockOffset := int64(ext3fs.SuperBlock.S_block_start + (indirectBlockNum * ext3fs.SuperBlock.S_block_s))
			err := utilitiesP2.ReadStruct(file, indirectBlockOffset, indirectBlock)
			if err == nil {
				// Buscar en cada bloque apuntado
				for _, blockNum := range indirectBlock.B_pointers {
					if blockNum == -1 || found {
						continue
					}

					dirBlock := &BloquesP2.BloqueCarpeta{}
					blockOffset := int64(ext3fs.SuperBlock.S_block_start + (blockNum * ext3fs.SuperBlock.S_block_s))
					err := utilitiesP2.ReadStruct(file, blockOffset, dirBlock)
					if err != nil {
						continue
					}

					for j := 0; j < 4; j++ {
						entry := dirBlock.B_content[j]
						if entry.B_inodo == -1 {
							continue
						}

						entryName := strings.TrimRight(string(entry.B_name[:]), "\x00")
						if entryName == component {
							currentInodeNum = entry.B_inodo
							found = true
							break
						}
					}
				}
			}
		}

		if !found {
			return -1, fmt.Errorf("componente de ruta no encontrado: %s", component)
		}
	}

	return currentInodeNum, nil
}

func createFileSystemItem(name, basePath string, inode *estructuraArchivoCarpeta.Inodo, showContent bool) FileSystemItem {
	// Construir ruta completa
	var fullPath string
	if basePath == "/" {
		fullPath = "/" + name
	} else {
		fullPath = basePath + "/" + name
	}

	// Determinar si es directorio
	isDirectory := inode.IsDirectory()

	// Obtener permisos usando la función existente del inodo
	permissions := inode.GetPermissionsString()

	// Obtener propietario y grupo
	owner := getOwnerName(inode.GetIuid())
	group := getGroupName(inode.GetIgid())

	return FileSystemItem{
		Name:        name,
		Path:        fullPath,
		IsDirectory: isDirectory,
		Size:        int64(inode.Is),
		Permissions: permissions,
		Owner:       owner,
		Group:       group,
		Modified:    "", // TODO: Agregar fecha de modificación si está disponible
	}
}

// getOwnerName obtiene el nombre del propietario por UID
func getOwnerName(uid int32) string {
	if uid == 1 {
		return "root"
	}
	return fmt.Sprintf("user%d", uid)
}

// getGroupName obtiene el nombre del grupo por GID
func getGroupName(gid int32) string {
	if gid == 1 {
		return "root"
	}
	return fmt.Sprintf("group%d", gid)
}

// readFileContentFromInodeDirectly lee contenido de archivo directamente desde el inodo
func readFileContentFromInodeDirectly(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, inode *estructuraArchivoCarpeta.Inodo) (string, error) {
	if !inode.IsFile() {
		return "", fmt.Errorf("el inodo no es un archivo")
	}

	// Usar la función existente de cat.go
	content, err := readFileContentFromBlocks(file, ext2fs, inode)
	if err != nil {
		return "", err
	}

	return content, nil
}

// En FileSystems.go, agregar nuevo endpoint para contenido de archivos usando CAT

// HandleFileContentAPI maneja las peticiones para obtener contenido de archivos usando CAT
func HandleFileContentAPI(w http.ResponseWriter, r *http.Request) {
	// Permitir CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		return
	}

	if r.Method != "GET" {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	// Obtener parámetros
	partitionID := r.URL.Query().Get("partition_id")
	filePath := r.URL.Query().Get("file_path")

	if partitionID == "" || filePath == "" {
		response := map[string]interface{}{
			"success": false,
			"error":   "Parámetros 'partition_id' y 'file_path' son requeridos",
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Verificar sesión activa
	if !sessionP2.IsSessionActive() {
		response := map[string]interface{}{
			"success": false,
			"error":   "No hay sesión activa",
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	session := sessionP2.GetCurrentSession()
	if session.PartitionID != partitionID {
		response := map[string]interface{}{
			"success": false,
			"error":   "La sesión actual no corresponde a la partición solicitada",
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Usar las funciones de CAT para leer el archivo
	content, fileInfo, err := readFileUsingCAT(partitionID, filePath)
	if err != nil {
		response := map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Error leyendo archivo: %s", err.Error()),
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	response := map[string]interface{}{
		"success":  true,
		"content":  content,
		"fileInfo": fileInfo,
		"message":  "Archivo leído exitosamente usando CAT",
	}

	json.NewEncoder(w).Encode(response)
}

// readFileUsingCAT usa las funciones existentes de CAT para leer archivos
func readFileUsingCAT(partitionID, filePath string) (string, FileSystemItem, error) {
	// Obtener la partición montada
	mountedPartition, exists := GetMountedPartition(partitionID)
	if !exists {
		return "", FileSystemItem{}, fmt.Errorf("partición no montada")
	}

	// Obtener información de la sesión
	sessionInfo := sessionP2.GetCurrentSession()

	// Usar ReadFileWithPermissions que ya implementaste en cat.go
	content, err := ReadFileWithPermissions(mountedPartition, filePath, sessionInfo)
	if err != nil {
		return "", FileSystemItem{}, err
	}

	// Obtener información del archivo para el FileSystemItem
	fileInfo, err := getFileInfoFromPath(mountedPartition, filePath, sessionInfo)
	if err != nil {
		return "", FileSystemItem{}, err
	}

	return content, fileInfo, nil
}

// getFileInfoFromPath obtiene información de un archivo usando las funciones existentes
func getFileInfoFromPath(partition MountedPartition, filePath string, session *sessionP2.UserSession) (FileSystemItem, error) {
	// Detectar tipo de sistema de archivos
	fsType, err := DetectFileSystemType(partition)
	if err != nil {
		return FileSystemItem{}, err
	}

	switch fsType {
	case estructuraArchivoCarpeta.EXT3_FILESYSTEM_TYPE:
		return getFileInfoEXT3(partition, filePath, session)
	case estructuraArchivoCarpeta.EXT2_FILESYSTEM_TYPE:
		return getFileInfoEXT2(partition, filePath, session)
	default:
		return FileSystemItem{}, fmt.Errorf("sistema de archivos no soportado: %d", fsType)
	}
}

// getFileInfoEXT3 obtiene información de archivo en EXT3
func getFileInfoEXT3(partition MountedPartition, filePath string, session *sessionP2.UserSession) (FileSystemItem, error) {
	// Abrir el archivo del disco
	file, err := os.OpenFile(partition.Path, os.O_RDONLY, 0666)
	if err != nil {
		return FileSystemItem{}, fmt.Errorf("error abriendo disco: %v", err)
	}
	defer file.Close()

	// Leer el sistema EXT3
	ext3fs, err := EstructuraExt3.ReadEXT3FileSystem(partition.Path, partition.Start)
	if err != nil {
		return FileSystemItem{}, fmt.Errorf("error leyendo sistema EXT3: %v", err)
	}

	// Encontrar el inodo del archivo
	fileInodeNum, err := findDirectoryByPathEXT3(file, ext3fs, filePath)
	if err != nil {
		return FileSystemItem{}, fmt.Errorf("archivo no encontrado: %v", err)
	}

	// Leer el inodo del archivo
	fileInode, err := ext3fs.LeerInodo(fileInodeNum)
	if err != nil {
		return FileSystemItem{}, fmt.Errorf("error leyendo inodo del archivo: %v", err)
	}

	// Verificar que es un archivo
	if !fileInode.IsFile() {
		return FileSystemItem{}, fmt.Errorf("la ruta especificada no es un archivo")
	}

	// Crear información del archivo
	fileName := getFileNameFromPath(filePath)
	fileInfo := createFileSystemItem(fileName, getParentPath(filePath), fileInode, false)

	return fileInfo, nil
}

// getFileInfoEXT2 obtiene información de archivo en EXT2
func getFileInfoEXT2(partition MountedPartition, filePath string, session *sessionP2.UserSession) (FileSystemItem, error) {
	// Abrir el archivo del disco
	file, err := os.OpenFile(partition.Path, os.O_RDONLY, 0666)
	if err != nil {
		return FileSystemItem{}, fmt.Errorf("error abriendo disco: %v", err)
	}
	defer file.Close()

	// Leer el sistema EXT2
	ext2fs, err := EstructuraExt2.ReadEXT2FileSystem(partition.Path, partition.Start)
	if err != nil {
		return FileSystemItem{}, fmt.Errorf("error leyendo sistema EXT2: %v", err)
	}

	// Buscar el inodo del archivo usando la función de cat.go
	fileInodeNum, err := findFileByPath(file, ext2fs, filePath)
	if err != nil {
		return FileSystemItem{}, fmt.Errorf("archivo no encontrado: %v", err)
	}

	// Leer el inodo del archivo
	fileInode := &estructuraArchivoCarpeta.Inodo{}
	inodeOffset := int64(ext2fs.SuperBlock.S_inode_start + (fileInodeNum * ext2fs.SuperBlock.S_inode_s))
	err = utilitiesP2.ReadStruct(file, inodeOffset, fileInode)
	if err != nil {
		return FileSystemItem{}, fmt.Errorf("error leyendo inodo del archivo: %v", err)
	}

	// Verificar que es un archivo
	if !fileInode.IsFile() {
		return FileSystemItem{}, fmt.Errorf("la ruta especificada no es un archivo")
	}

	// Crear información del archivo
	fileName := getFileNameFromPath(filePath)
	fileInfo := createFileSystemItem(fileName, getParentPath(filePath), fileInode, false)

	return fileInfo, nil
}

// Funciones auxiliares (si no las tienes ya)
func getFileNameFromPath(filePath string) string {
	parts := strings.Split(strings.Trim(filePath, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func getParentPath(filePath string) string {
	parts := strings.Split(strings.Trim(filePath, "/"), "/")
	if len(parts) <= 1 {
		return "/"
	}
	return "/" + strings.Join(parts[:len(parts)-1], "/")
}
