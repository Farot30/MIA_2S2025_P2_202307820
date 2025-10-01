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
	"regexp"
	"sort"
	"strings"
)

func FIND(args []string) {
	// Verificar que hay una sesión activa
	if !sessionP2.IsSessionActive() {
		fmt.Printf("ERROR: No active session. Please log in first.\n")
		return
	}

	// Parsear parámetros
	params := utilitiesP2.ParseParameters(args)
	searchPath := strings.Trim(params["path"], "\"")
	searchName := strings.Trim(params["name"], "\"")

	// Validar parámetros obligatorios
	if searchPath == "" {
		fmt.Println("ERROR: Falta el parámetro obligatorio 'path'.")
		fmt.Println("Uso: find -path=<ruta_busqueda> -name=<patron_nombre>")
		return
	}

	if searchName == "" {
		fmt.Println("ERROR: Falta el parámetro obligatorio 'name'.")
		fmt.Println("Uso: find -path=<ruta_busqueda> -name=<patron_nombre>")
		return
	}

	// Validar que la ruta sea absoluta
	if !strings.HasPrefix(searchPath, "/") {
		fmt.Println("ERROR: La ruta debe ser absoluta y comenzar con '/'.")
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

	// Determinar tipo de sistema de archivos y ejecutar búsqueda
	fileSystemType, err := getFileSystemTypeForFind(mountedPartition)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}

	fmt.Printf("=== BÚSQUEDA DE ARCHIVOS ===\n")
	fmt.Printf("Ruta de búsqueda: %s\n", searchPath)
	fmt.Printf("Patrón de búsqueda: %s\n", searchName)
	fmt.Printf("Sistema de archivos: %s\n", getFileSystemTypeName(fileSystemType))
	fmt.Printf("=============================\n\n")

	// Ejecutar búsqueda según el tipo de sistema
	var results []FindResult
	if fileSystemType == EstructuraarchivocarpetaP2.EXT2_FILESYSTEM_TYPE {
		results, err = findInEXT2(mountedPartition, searchPath, searchName, sessionInfo)
	} else if fileSystemType == EstructuraarchivocarpetaP2.EXT3_FILESYSTEM_TYPE {
		results, err = findInEXT3(mountedPartition, searchPath, searchName, sessionInfo)
	} else {
		fmt.Println("ERROR: Tipo de sistema de archivos no soportado.")
		return
	}

	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}

	// Mostrar resultados
	displayFindResults(results, searchName)
}

// FindResult representa un resultado de búsqueda
type FindResult struct {
	Path        string
	Name        string
	IsDirectory bool
	Size        int32
	Permissions [3]byte
	UID         int32
	GID         int32
	FullPath    string
}

// getFileSystemTypeForFind determina el tipo de sistema de archivos
func getFileSystemTypeForFind(partition MountedPartition) (int32, error) {
	file, err := os.Open(partition.Path)
	if err != nil {
		return 0, fmt.Errorf("error abriendo la partición: %v", err)
	}
	defer file.Close()

	// Leer el superbloque
	superBlock := &EstructuraarchivocarpetaP2.SuperBloque{}
	startOffset := getPartitionStartOffsetForFind(partition)
	err = utilitiesP2.ReadStruct(file, int64(startOffset), superBlock)
	if err != nil {
		return 0, fmt.Errorf("error leyendo superbloque: %v", err)
	}

	return superBlock.S_filesystem_type, nil
}

// getPartitionStartOffsetForFind obtiene el offset de inicio de la partición
func getPartitionStartOffsetForFind(partition MountedPartition) int32 {
	// Reutilizar la función existente del login
	return getPartitionStartOffset(partition)
}

// getFileSystemTypeName convierte el tipo numérico a nombre
func getFileSystemTypeName(fsType int32) string {
	switch fsType {
	case EstructuraarchivocarpetaP2.EXT2_FILESYSTEM_TYPE:
		return "EXT2"
	case EstructuraarchivocarpetaP2.EXT3_FILESYSTEM_TYPE:
		return "EXT3"
	default:
		return "DESCONOCIDO"
	}
}

// findInEXT2 ejecuta la búsqueda en sistema EXT2
func findInEXT2(partition MountedPartition, searchPath, searchName string, sessionInfo *sessionP2.UserSession) ([]FindResult, error) {
	file, err := os.OpenFile(partition.Path, os.O_RDONLY, 0666)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir la partición: %v", err)
	}
	defer file.Close()

	ext2fs, err := EstructuraExt2P2.ReadEXT2FileSystem(partition.Path, getPartitionStartOffsetForFind(partition))
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer el sistema EXT2: %v", err)
	}

	return executeFind(file, ext2fs.SuperBlock, searchPath, searchName, sessionInfo, nil)
}

// findInEXT3 ejecuta la búsqueda en sistema EXT3 con journaling
func findInEXT3(partition MountedPartition, searchPath, searchName string, sessionInfo *sessionP2.UserSession) ([]FindResult, error) {
	file, err := os.OpenFile(partition.Path, os.O_RDONLY, 0666)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir la partición: %v", err)
	}
	defer file.Close()

	ext3fs, err := EstructuraExt3P2.ReadEXT3FileSystem(partition.Path, getPartitionStartOffsetForFind(partition))
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer el sistema EXT3: %v", err)
	}

	// Registrar operación de búsqueda en el journal
	searchInfo := fmt.Sprintf("path=%s name=%s user=%s", searchPath, searchName, sessionInfo.Username)
	err = ext3fs.AddJournalEntry(EstructuraExt3P2.OPERATION_RECOVERY, searchPath, "find_start: "+searchInfo)
	if err != nil {
		fmt.Printf("WARNING: No se pudo registrar inicio de búsqueda en journal: %v\n", err)
	}

	// Ejecutar búsqueda
	results, err := executeFind(file, ext3fs.SuperBlock, searchPath, searchName, sessionInfo, ext3fs)

	// Registrar resultado en journal
	resultInfo := fmt.Sprintf("found %d items", len(results))
	if err != nil {
		resultInfo = "error: " + err.Error()
	}

	err2 := ext3fs.AddJournalEntry(EstructuraExt3P2.OPERATION_RECOVERY, searchPath, "find_end: "+resultInfo)
	if err2 != nil {
		fmt.Printf("WARNING: No se pudo registrar fin de búsqueda en journal: %v\n", err2)
	}

	return results, err
}

// executeFind ejecuta la búsqueda común para EXT2 y EXT3
func executeFind(file *os.File, superBlock *EstructuraarchivocarpetaP2.SuperBloque, searchPath, searchName string, sessionInfo *sessionP2.UserSession, ext3fs *EstructuraExt3P2.EXT3FileSystem) ([]FindResult, error) {
	// Buscar el directorio de inicio
	startDirInodeNum, err := findDirectoryInEXT2ForFind(file, superBlock, searchPath)
	if err != nil {
		return nil, fmt.Errorf("no se pudo encontrar el directorio '%s': %v", searchPath, err)
	}

	// Leer el inodo del directorio de inicio
	startDirInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(superBlock.S_inode_start+startDirInodeNum*superBlock.S_inode_s), startDirInode)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer inodo del directorio de inicio: %v", err)
	}

	// Verificar permisos de lectura en el directorio de inicio
	canRead, err := sessionP2.CheckPermissions(startDirInode, sessionP2.READ)
	if err != nil || !canRead {
		return nil, fmt.Errorf("no tiene permisos de lectura en el directorio '%s'", searchPath)
	}

	// Convertir patrón a expresión regular
	pattern, err := convertWildcardToRegex(searchName)
	if err != nil {
		return nil, fmt.Errorf("patrón de búsqueda inválido '%s': %v", searchName, err)
	}

	// Ejecutar búsqueda recursiva
	var results []FindResult
	err = searchRecursively(file, superBlock, startDirInode, startDirInodeNum, searchPath, pattern, sessionInfo, ext3fs, &results)
	if err != nil {
		return nil, err
	}

	// Ordenar resultados por ruta completa
	sort.Slice(results, func(i, j int) bool {
		return results[i].FullPath < results[j].FullPath
	})

	return results, nil
}

// convertWildcardToRegex convierte un patrón con wildcards a expresión regular
func convertWildcardToRegex(pattern string) (*regexp.Regexp, error) {
	// Escapar caracteres especiales de regex excepto * y ?
	escaped := regexp.QuoteMeta(pattern)

	// Reemplazar los wildcards escapados por sus equivalentes regex
	escaped = strings.ReplaceAll(escaped, "\\*", ".*") // * = uno o más caracteres
	escaped = strings.ReplaceAll(escaped, "\\?", ".")  // ? = un solo carácter

	// Anclar al inicio y fin para coincidencia exacta
	regexPattern := "^" + escaped + "$"

	return regexp.Compile(regexPattern)
}

// searchRecursively busca recursivamente en un directorio
func searchRecursively(file *os.File, superBlock *EstructuraarchivocarpetaP2.SuperBloque, dirInode *EstructuraarchivocarpetaP2.Inodo, dirInodeNum int32, currentPath string, pattern *regexp.Regexp, sessionInfo *sessionP2.UserSession, ext3fs *EstructuraExt3P2.EXT3FileSystem, results *[]FindResult) error {
	// Verificar permisos de lectura
	canRead, err := sessionP2.CheckPermissions(dirInode, sessionP2.READ)
	if err != nil || !canRead {
		fmt.Printf("[WARNING] Sin permisos de lectura en '%s'\n", currentPath)
		return nil // Continuar con otros directorios
	}

	// Procesar bloques directos del directorio
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		block := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(superBlock.S_block_start+blockNum*superBlock.S_block_s), block)
		if err != nil {
			fmt.Printf("[WARNING] No se pudo leer bloque %d: %v\n", blockNum, err)
			continue
		}

		// Procesar cada entrada del bloque
		for j := 0; j < 4; j++ {
			if !block.IsEntryEmpty(j) {
				entryName, entryInodeNum, _ := block.GetEntry(j)

				// Saltar entradas . y ..
				if entryName == "." || entryName == ".." {
					continue
				}

				// Construir ruta completa
				entryPath := filepath.Join(currentPath, entryName)
				if currentPath == "/" {
					entryPath = "/" + entryName
				}

				// Leer el inodo de la entrada
				entryInode := &EstructuraarchivocarpetaP2.Inodo{}
				err := utilitiesP2.ReadStruct(file, int64(superBlock.S_inode_start+entryInodeNum*superBlock.S_inode_s), entryInode)
				if err != nil {
					fmt.Printf("[WARNING] No se pudo leer inodo de '%s': %v\n", entryName, err)
					continue
				}

				// Verificar si el nombre coincide con el patrón
				if pattern.MatchString(entryName) {
					// Agregar a resultados
					result := FindResult{
						Path:        currentPath,
						Name:        entryName,
						IsDirectory: entryInode.Itype == 0,
						Size:        entryInode.Is,
						Permissions: entryInode.Iperm,
						UID:         entryInode.Iuid,
						GID:         entryInode.Igid,
						FullPath:    entryPath,
					}
					*results = append(*results, result)

					// Registrar coincidencia en journal si es EXT3
					if ext3fs != nil {
						matchInfo := fmt.Sprintf("match: %s type=%s size=%d", entryPath, getEntryType(int32(entryInode.Itype)), entryInode.Is)
						ext3fs.AddJournalEntry("find_match", entryPath, matchInfo)
					}
				}

				// Si es directorio, buscar recursivamente
				if entryInode.Itype == 0 {
					err = searchRecursively(file, superBlock, entryInode, entryInodeNum, entryPath, pattern, sessionInfo, ext3fs, results)
					if err != nil {
						fmt.Printf("[WARNING] Error buscando en subdirectorio '%s': %v\n", entryPath, err)
					}
				}
			}
		}
	}

	// Procesar bloques indirectos si existen
	if dirInode.Iblock[12] != -1 {
		err = searchInIndirectBlocks(file, superBlock, dirInode.Iblock[12], currentPath, pattern, sessionInfo, ext3fs, results)
		if err != nil {
			fmt.Printf("[WARNING] Error buscando en bloques indirectos de '%s': %v\n", currentPath, err)
		}
	}

	return nil
}

// searchInIndirectBlocks busca en bloques indirectos
func searchInIndirectBlocks(file *os.File, superBlock *EstructuraarchivocarpetaP2.SuperBloque, indirectBlockNum int32, currentPath string, pattern *regexp.Regexp, sessionInfo *sessionP2.UserSession, ext3fs *EstructuraExt3P2.EXT3FileSystem, results *[]FindResult) error {
	// Leer el bloque de apuntadores
	pointerBlock := &BloquesP2.BloqueApuntador{}
	err := utilitiesP2.ReadStruct(file, int64(superBlock.S_block_start+indirectBlockNum*superBlock.S_block_s), pointerBlock)
	if err != nil {
		return fmt.Errorf("error leyendo bloque indirecto: %v", err)
	}

	// Procesar cada apuntador
	for i := 0; i < 16; i++ {
		blockNum := pointerBlock.GetPointer(i)
		if blockNum == -1 {
			continue
		}

		block := &BloquesP2.BloqueCarpeta{}
		err := utilitiesP2.ReadStruct(file, int64(superBlock.S_block_start+blockNum*superBlock.S_block_s), block)
		if err != nil {
			continue
		}

		// Procesar entradas del bloque igual que en bloques directos
		for j := 0; j < 4; j++ {
			if !block.IsEntryEmpty(j) {
				entryName, entryInodeNum, _ := block.GetEntry(j)

				if entryName == "." || entryName == ".." {
					continue
				}

				entryPath := filepath.Join(currentPath, entryName)
				if currentPath == "/" {
					entryPath = "/" + entryName
				}

				entryInode := &EstructuraarchivocarpetaP2.Inodo{}
				err := utilitiesP2.ReadStruct(file, int64(superBlock.S_inode_start+entryInodeNum*superBlock.S_inode_s), entryInode)
				if err != nil {
					continue
				}

				if pattern.MatchString(entryName) {
					result := FindResult{
						Path:        currentPath,
						Name:        entryName,
						IsDirectory: entryInode.Itype == 0,
						Size:        entryInode.Is,
						Permissions: entryInode.Iperm,
						UID:         entryInode.Iuid,
						GID:         entryInode.Igid,
						FullPath:    entryPath,
					}
					*results = append(*results, result)

					if ext3fs != nil {
						matchInfo := fmt.Sprintf("match: %s type=%s size=%d", entryPath, getEntryType(int32(entryInode.Itype)), entryInode.Is)
						ext3fs.AddJournalEntry("find_match", entryPath, matchInfo)
					}
				}

				if entryInode.Itype == 0 {
					err = searchRecursively(file, superBlock, entryInode, entryInodeNum, entryPath, pattern, sessionInfo, ext3fs, results)
					if err != nil {
						fmt.Printf("[WARNING] Error buscando en subdirectorio '%s': %v\n", entryPath, err)
					}
				}
			}
		}
	}

	return nil
}

// getEntryType convierte el tipo numérico a string
func getEntryType(itype int32) string {
	if itype == 0 {
		return "directory"
	} else if itype == 1 {
		return "file"
	}
	return "unknown"
}

// displayFindResults muestra los resultados de la búsqueda
func displayFindResults(results []FindResult, searchPattern string) {
	if len(results) == 0 {
		fmt.Printf("No se encontraron archivos o carpetas que coincidan con el patrón '%s'\n", searchPattern)
		return
	}

	fmt.Printf("RESULTADOS DE BÚSQUEDA:\n")
	fmt.Printf("========================\n")
	fmt.Printf("Patrón: %s\n", searchPattern)
	fmt.Printf("Elementos encontrados: %d\n\n", len(results))

	// Agrupar por directorio padre para mostrar como árbol
	directoryGroups := make(map[string][]FindResult)
	for _, result := range results {
		directoryGroups[result.Path] = append(directoryGroups[result.Path], result)
	}

	// Ordenar directorios
	var sortedDirs []string
	for dir := range directoryGroups {
		sortedDirs = append(sortedDirs, dir)
	}
	sort.Strings(sortedDirs)

	// Mostrar resultados agrupados por directorio
	for _, dir := range sortedDirs {
		fmt.Printf("%s/\n", dir)

		entries := directoryGroups[dir]
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Name < entries[j].Name
		})

		for _, entry := range entries {
			entryType := "📄"
			if entry.IsDirectory {
				entryType = "📁"
			}

			permissions := sessionP2.GetPermissionsString(entry.Permissions)
			fmt.Printf("  %s %s %s (UID:%d GID:%d Size:%d)\n",
				entryType, entry.Name, permissions, entry.UID, entry.GID, entry.Size)
		}
		fmt.Println()
	}

	fmt.Printf("Total de elementos encontrados: %d\n", len(results))
}

// findDirectoryInEXT2ForFind busca un directorio en el sistema EXT2
func findDirectoryInEXT2ForFind(file *os.File, superBlock *EstructuraarchivocarpetaP2.SuperBloque, path string) (int32, error) {
	if path == "/" {
		return 0, nil
	}

	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return 0, nil
	}

	components := strings.Split(strings.TrimPrefix(path, "/"), "/")
	currentInodeNum := int32(0)

	for _, component := range components {
		if component == "" {
			continue
		}

		currentInode := &EstructuraarchivocarpetaP2.Inodo{}
		err := utilitiesP2.ReadStruct(file, int64(superBlock.S_inode_start+currentInodeNum*superBlock.S_inode_s), currentInode)
		if err != nil {
			return -1, fmt.Errorf("error leyendo inodo del directorio: %v", err)
		}

		if currentInode.Itype != 0 {
			return -1, fmt.Errorf("'%s' no es un directorio", component)
		}

		nextInodeNum, err := findFileInDirectoryEXT2ForFind(file, superBlock, currentInode, component)
		if err != nil || nextInodeNum == -1 {
			return -1, fmt.Errorf("no se encontró '%s' en el directorio", component)
		}

		currentInodeNum = nextInodeNum
	}

	return currentInodeNum, nil
}

// findFileInDirectoryEXT2ForFind busca un archivo o directorio en un directorio específico
func findFileInDirectoryEXT2ForFind(file *os.File, superBlock *EstructuraarchivocarpetaP2.SuperBloque, dirInode *EstructuraarchivocarpetaP2.Inodo, fileName string) (int32, error) {
	if dirInode.Itype != 0 {
		return -1, fmt.Errorf("el inodo no es un directorio")
	}

	// Buscar en bloques directos
	for i := 0; i < 12 && dirInode.Iblock[i] != -1; i++ {
		blockNum := dirInode.Iblock[i]
		block := &BloquesP2.BloqueCarpeta{}

		err := utilitiesP2.ReadStruct(file, int64(superBlock.S_block_start+blockNum*superBlock.S_block_s), block)
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
