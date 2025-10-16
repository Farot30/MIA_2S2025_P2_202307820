package commandsP2

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DiskRegistryEntry representa un disco registrado
type DiskRegistryEntry struct {
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	Size         int32     `json:"size"`
	Fit          string    `json:"fit"`
	CreatedAt    time.Time `json:"created_at"`
	LastAccessed time.Time `json:"last_accessed"`
}

// DiskRegistry maneja el registro de discos creados
type DiskRegistry struct {
	Disks []DiskRegistryEntry `json:"disks"`
	mutex sync.RWMutex
}

var diskRegistry = &DiskRegistry{
	Disks: make([]DiskRegistryEntry, 0),
}

const REGISTRY_FILE = "./disk_registry.json"

// RegisterDisk registra un nuevo disco en el registro
func RegisterDisk(path string, size int32, fit byte) error {
	diskRegistry.mutex.Lock()
	defer diskRegistry.mutex.Unlock()

	// Verificar que el archivo existe
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("el disco no existe en la ruta especificada: %s", path)
	}

	// Obtener ruta absoluta
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("error obteniendo ruta absoluta: %v", err)
	}

	// Verificar si ya está registrado
	for i, disk := range diskRegistry.Disks {
		if disk.Path == absPath {
			// Actualizar entrada existente
			diskRegistry.Disks[i].LastAccessed = time.Now()
			return saveRegistry()
		}
	}

	// Crear nueva entrada
	diskName := filepath.Base(path)
	diskName = diskName[:len(diskName)-len(filepath.Ext(diskName))] // Remover extensión

	entry := DiskRegistryEntry{
		Name:         diskName,
		Path:         absPath,
		Size:         size,
		Fit:          string(fit),
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
	}

	diskRegistry.Disks = append(diskRegistry.Disks, entry)

	err = saveRegistry()
	if err != nil {
		return fmt.Errorf("error guardando registro: %v", err)
	}

	fmt.Printf("✅ Disco registrado exitosamente en el sistema\n")
	return nil
}

// UnregisterDisk elimina un disco del registro
func UnregisterDisk(path string) error {
	diskRegistry.mutex.Lock()
	defer diskRegistry.mutex.Unlock()

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("error obteniendo ruta absoluta: %v", err)
	}

	for i, disk := range diskRegistry.Disks {
		if disk.Path == absPath {
			// Eliminar de la lista
			diskRegistry.Disks = append(diskRegistry.Disks[:i], diskRegistry.Disks[i+1:]...)
			return saveRegistry()
		}
	}

	return fmt.Errorf("disco no encontrado en el registro")
}

// GetRegisteredDisks obtiene todos los discos registrados
func GetRegisteredDisks() ([]DiskRegistryEntry, error) {
	diskRegistry.mutex.RLock()
	defer diskRegistry.mutex.RUnlock()

	// Cargar registro si no está cargado
	if err := loadRegistry(); err != nil {
		return nil, err
	}

	// Verificar que los archivos aún existen
	var validDisks []DiskRegistryEntry
	for _, disk := range diskRegistry.Disks {
		if _, err := os.Stat(disk.Path); err == nil {
			validDisks = append(validDisks, disk)
		}
	}

	// Si hay diferencias, actualizar el registro
	if len(validDisks) != len(diskRegistry.Disks) {
		diskRegistry.Disks = validDisks
		saveRegistry()
	}

	return validDisks, nil
}

// saveRegistry guarda el registro en disco
func saveRegistry() error {
	data, err := json.MarshalIndent(diskRegistry, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(REGISTRY_FILE, data, 0644)
}

// loadRegistry carga el registro desde disco
func loadRegistry() error {
	if _, err := os.Stat(REGISTRY_FILE); os.IsNotExist(err) {
		// Si no existe, crear registro vacío
		return saveRegistry()
	}

	data, err := os.ReadFile(REGISTRY_FILE)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, diskRegistry)
}

// UpdateLastAccessed actualiza el último acceso de un disco
func UpdateLastAccessed(path string) {
	diskRegistry.mutex.Lock()
	defer diskRegistry.mutex.Unlock()

	absPath, err := filepath.Abs(path)
	if err != nil {
		return
	}

	for i, disk := range diskRegistry.Disks {
		if disk.Path == absPath {
			diskRegistry.Disks[i].LastAccessed = time.Now()
			saveRegistry()
			return
		}
	}
}

// ListRegisteredDisks muestra todos los discos registrados
func ListRegisteredDisks() {
	disks, err := GetRegisteredDisks()
	if err != nil {
		fmt.Printf("Error obteniendo discos registrados: %v\n", err)
		return
	}

	if len(disks) == 0 {
		fmt.Println("No hay discos registrados en el sistema")
		return
	}

	fmt.Printf("\n=== DISCOS REGISTRADOS ===\n")
	fmt.Printf("%-20s | %-50s | %-10s | %-8s | %-20s\n", "NOMBRE", "RUTA", "TAMAÑO", "FIT", "CREADO")
	fmt.Println(strings.Repeat("-", 120))

	for _, disk := range disks {
		sizeStr := formatSize(disk.Size)
		fmt.Printf("%-20s | %-50s | %-10s | %-8s | %-20s\n",
			disk.Name,
			disk.Path,
			sizeStr,
			disk.Fit,
			disk.CreatedAt.Format("2006-01-02 15:04"))
	}
	fmt.Printf("Total: %d discos registrados\n", len(disks))
}

func formatSize(bytes int32) string {
	if bytes >= 1024*1024*1024 {
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1024*1024*1024))
	} else if bytes >= 1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	} else if bytes >= 1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%d B", bytes)
}
