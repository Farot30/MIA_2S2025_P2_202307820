package commandsP2

import (
	structuresP2 "MIA_2S2025_P2_202307820/structures"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type DiskInfo struct {
	Name           string          `json:"name"`
	Path           string          `json:"path"`
	Size           int32           `json:"size"`
	Fit            string          `json:"fit"`
	Signature      int32           `json:"signature"`
	CreationDate   string          `json:"creation_date"`
	Partitions     []PartitionInfo `json:"partitions"`
	MountedCount   int             `json:"mounted_count"`
	PartitionTypes map[string]int  `json:"partition_types"`
}

type PartitionInfo struct {
	Index      int    `json:"index"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	Size       int32  `json:"size"`
	Start      int32  `json:"start"`
	Fit        string `json:"fit"`
	IsMounted  bool   `json:"is_mounted"`
	MountedID  string `json:"mounted_id,omitempty"`
	FileSystem string `json:"file_system,omitempty"`
	FreeSpace  int32  `json:"free_space"`
	UsedSpace  int32  `json:"used_space"`
}

// HandleDisksAPI maneja las peticiones API para obtener información de discos
func HandleDisksAPI(w http.ResponseWriter, r *http.Request) {
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

	// Obtener información de particiones montadas
	mountedPartitions := GetAllMountedPartitions()

	// Buscar archivos .mia en directorios comunes
	diskPaths := findDiskFiles()

	var disks []DiskInfo
	for _, diskPath := range diskPaths {
		diskInfo, err := getDiskInformation(diskPath, mountedPartitions)
		if err != nil {
			continue // Saltar discos que no se pueden leer
		}
		disks = append(disks, diskInfo)
	}

	response := map[string]interface{}{
		"success": true,
		"disks":   disks,
		"total":   len(disks),
	}

	json.NewEncoder(w).Encode(response)
}

// findDiskFiles busca archivos .mia en directorios comunes
func findDiskFiles() []string {
	var diskPaths []string

	// Directorios donde buscar discos
	searchDirs := []string{
		"./",
		"../",
		"./discos/",
		"./disks/",
		"/home",
		"/tmp",
	}

	for _, dir := range searchDirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.mia"))
		if err == nil {
			diskPaths = append(diskPaths, files...)
		}
	}

	// Eliminar duplicados
	uniquePaths := make(map[string]bool)
	var result []string
	for _, path := range diskPaths {
		absPath, err := filepath.Abs(path)
		if err == nil && !uniquePaths[absPath] {
			uniquePaths[absPath] = true
			result = append(result, absPath)
		}
	}

	return result
}

// getDiskInformation obtiene información detallada de un disco
func getDiskInformation(diskPath string, mountedPartitions map[string]MountedPartition) (DiskInfo, error) {
	file, err := os.OpenFile(diskPath, os.O_RDONLY, 0666)
	if err != nil {
		return DiskInfo{}, err
	}
	defer file.Close()

	// Leer MBR
	var mbr structuresP2.MBR
	if err := utilitiesP2.ReadStruct(file, 0, &mbr); err != nil {
		return DiskInfo{}, err
	}

	diskName := filepath.Base(diskPath)
	diskName = strings.TrimSuffix(diskName, ".mia")

	diskInfo := DiskInfo{
		Name:           diskName,
		Path:           diskPath,
		Size:           mbr.MbrTamano,
		Fit:            string(mbr.DskFit),
		Signature:      mbr.MbrDskSignature,
		CreationDate:   strings.TrimRight(string(mbr.MbrFechaCreacion[:]), "\x00"),
		Partitions:     []PartitionInfo{},
		PartitionTypes: make(map[string]int),
	}

	// Analizar particiones
	mountedCount := 0
	for i, partition := range mbr.MbrPartitions {
		if partition.PartS == 0 {
			continue // Partición vacía
		}

		partName := strings.TrimRight(string(partition.PartName[:]), "\x00")
		partType := getPartitionTypeName(partition.PartType)
		partStatus := getPartitionStatusName(partition.PartStatus)

		// Verificar si está montada
		var mountedID string
		var fileSystem string
		isMounted := false
		for id, mp := range mountedPartitions {
			if strings.Contains(mp.Path, diskName) && mp.Name == partName {
				isMounted = true
				mountedID = id
				fileSystem = mp.Type
				mountedCount++
				break
			}
		}

		partInfo := PartitionInfo{
			Index:      i + 1,
			Name:       partName,
			Type:       partType,
			Status:     partStatus,
			Size:       partition.PartS,
			Start:      partition.PartStart,
			Fit:        string(partition.PartFit),
			IsMounted:  isMounted,
			MountedID:  mountedID,
			FileSystem: fileSystem,
		}

		diskInfo.Partitions = append(diskInfo.Partitions, partInfo)
		diskInfo.PartitionTypes[partType]++
	}

	diskInfo.MountedCount = mountedCount

	// Analizar particiones lógicas si hay una extendida
	for _, partition := range mbr.MbrPartitions {
		if partition.PartType == 'E' && partition.PartS > 0 {
			logicalPartitions := getLogicalPartitions(file, &partition, mountedPartitions, diskName)
			diskInfo.Partitions = append(diskInfo.Partitions, logicalPartitions...)
		}
	}

	return diskInfo, nil
}

// getLogicalPartitions obtiene información de particiones lógicas
func getLogicalPartitions(file *os.File, extendedPartition *structuresP2.Partition, mountedPartitions map[string]MountedPartition, diskName string) []PartitionInfo {
	var logicalPartitions []PartitionInfo

	currentEBRPos := extendedPartition.PartStart
	extendedEnd := extendedPartition.PartStart + extendedPartition.PartS
	logicalIndex := 5 // Las lógicas empiezan en 5

	for currentEBRPos != -1 && currentEBRPos < extendedEnd {
		ebr, err := structuresP2.ReadEBR(file, currentEBRPos)
		if err != nil || ebr.IsEmpty() {
			break
		}

		partName := ebr.GetName()

		// Verificar si está montada
		var mountedID string
		var fileSystem string
		isMounted := false
		for id, mp := range mountedPartitions {
			if strings.Contains(mp.Path, diskName) && mp.Name == partName {
				isMounted = true
				mountedID = id
				fileSystem = mp.Type
				break
			}
		}

		partInfo := PartitionInfo{
			Index:      logicalIndex,
			Name:       partName,
			Type:       "Lógica",
			Status:     "Activa",
			Size:       ebr.PartS,
			Start:      ebr.PartStart,
			Fit:        string(ebr.PartFit),
			IsMounted:  isMounted,
			MountedID:  mountedID,
			FileSystem: fileSystem,
		}

		logicalPartitions = append(logicalPartitions, partInfo)
		logicalIndex++

		if !ebr.HasNext() {
			break
		}
		currentEBRPos = ebr.PartNext
	}

	return logicalPartitions
}

// getPartitionTypeName convierte el tipo de partición a nombre legible
func getPartitionTypeName(partType byte) string {
	switch partType {
	case 'P':
		return "Primaria"
	case 'E':
		return "Extendida"
	case 'L':
		return "Lógica"
	default:
		return "Desconocida"
	}
}

// getPartitionStatusName convierte el estado de partición a nombre legible
func getPartitionStatusName(status byte) string {
	switch status {
	case '0':
		return "No montada"
	case '1':
		return "Montada"
	case '2':
		return "Formateada"
	default:
		return "Desconocido"
	}
}

// GetAllMountedPartitions obtiene todas las particiones montadas
// En diskInfo.go, corregir esta función:
func GetAllMountedPartitions() map[string]MountedPartition {
	result := make(map[string]MountedPartition)

	// Usar el mutex del mountManager en lugar de mountedPartitionsMutex
	mountManager.mutex.RLock()
	defer mountManager.mutex.RUnlock()

	for id, partition := range mountManager.mountedPartitions {
		result[id] = partition
	}

	return result
}
