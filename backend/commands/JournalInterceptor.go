package commandsP2

import (
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	sessionP2 "MIA_2S2025_P2_202307820/Session"
	"fmt"
	"sync"
	"time"
)

// JournalInterceptor maneja el registro automático de operaciones en EXT3
type JournalInterceptor struct {
	enabled       bool
	lastOperation map[string]time.Time // Para evitar duplicados
	mutex         sync.Mutex
}

var journalInterceptor = &JournalInterceptor{
	enabled:       true,
	lastOperation: make(map[string]time.Time),
}

// LogEXT3Operation registra una operación en el journal sin duplicados
func LogEXT3Operation(operation, path, details string) {
	if !journalInterceptor.enabled {
		return
	}

	journalInterceptor.mutex.Lock()
	defer journalInterceptor.mutex.Unlock()

	// Crear clave única para la operación
	opKey := fmt.Sprintf("%s:%s:%s", operation, path, details)

	// Verificar si ya se registró recientemente (últimos 2 segundos)
	if lastTime, exists := journalInterceptor.lastOperation[opKey]; exists {
		if time.Since(lastTime) < 2*time.Second {
			return // Evitar duplicado
		}
	}

	// Obtener la sesión activa
	if !sessionP2.IsSessionActive() {
		return
	}

	session := sessionP2.GetCurrentSession()
	if session == nil {
		return
	}

	// Buscar la partición montada correspondiente a la sesión
	mountedPartition, exists := GetMountedPartition(session.PartitionID)
	if !exists {
		return
	}

	// Verificar si es EXT3
	if !isEXT3Partition(mountedPartition) {
		return
	}

	// Obtener el filesystem EXT3
	ext3fs, err := EstructuraExt3P2.ReadEXT3FileSystem(mountedPartition.Path, mountedPartition.Start)
	if err != nil {
		fmt.Printf("WARNING: Error leyendo EXT3 para journal: %v\n", err)
		return
	}

	// Limpiar y preparar detalles
	cleanDetails := details
	if len(cleanDetails) > 60 {
		cleanDetails = cleanDetails[:60]
	}

	// Agregar información del usuario
	userDetails := fmt.Sprintf("user=%s %s", session.Username, cleanDetails)
	if len(userDetails) > 63 {
		userDetails = userDetails[:63]
	}

	// Registrar en el journal
	ext3fs.LogOperation(operation, path, userDetails)

	// Actualizar tiempo de última operación
	journalInterceptor.lastOperation[opKey] = time.Now()

	// Limpiar entradas antigas (más de 10 segundos)
	for key, t := range journalInterceptor.lastOperation {
		if time.Since(t) > 10*time.Second {
			delete(journalInterceptor.lastOperation, key)
		}
	}
}

// LogEXT3OperationSimple registra una operación simple sin duplicados
func LogEXT3OperationSimple(operation, path string) {
	LogEXT3Operation(operation, path, "")
}

// LogEXT3OperationWithUser registra una operación con información de usuario específica
func LogEXT3OperationWithUser(operation, path, username, details string) {
	finalDetails := fmt.Sprintf("user=%s %s", username, details)
	LogEXT3Operation(operation, path, finalDetails)
}

// isEXT3Partition verifica si una partición es EXT3
func isEXT3Partition(partition MountedPartition) bool {
	fsType, err := detectFileSystemTypeForJournaling(partition)
	if err != nil {
		return false
	}
	return fsType == 3 // EXT3_FILESYSTEM_TYPE
}

// EnableJournalInterceptor habilita/deshabilita el interceptor
func EnableJournalInterceptor(enabled bool) {
	journalInterceptor.mutex.Lock()
	defer journalInterceptor.mutex.Unlock()
	journalInterceptor.enabled = enabled
}

// ClearJournalCache limpia el cache de operaciones
func ClearJournalCache() {
	journalInterceptor.mutex.Lock()
	defer journalInterceptor.mutex.Unlock()
	journalInterceptor.lastOperation = make(map[string]time.Time)
}

// IsJournalEnabled verifica si el journal está habilitado
func IsJournalEnabled() bool {
	journalInterceptor.mutex.Lock()
	defer journalInterceptor.mutex.Unlock()
	return journalInterceptor.enabled
}
