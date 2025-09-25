package EstructuraExt3P2

import (
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"os"
	"sort"
	"strings"
	"time"
	"unsafe"
)

type Information struct {
	I_Operation [10]byte // Tipo de operación (10 caracteres)
	I_Path      [32]byte // Ruta del archivo o carpeta (32 caracteres)
	I_Date      float64  // Fecha y hora de la operación (timestamp float64)
	I_Content   [64]byte // Contenido adicional (64 caracteres)
}

type Journal struct {
	J_Count   int32       // Lleva el conteo del journal que es
	J_Content Information // Estructura de información del journal
}

const (
	// Tipos de operaciones para el journal
	OPERATION_MKDIR    = "mkdir"    // Crear directorio
	OPERATION_MKFILE   = "mkfile"   // Crear archivo
	OPERATION_RMDIR    = "rmdir"    // Eliminar directorio
	OPERATION_RMFILE   = "rmfile"   // Eliminar archivo
	OPERATION_EDIT     = "edit"     // Editar archivo
	OPERATION_RENAME   = "rename"   // Renombrar archivo o directorio
	OPERATION_CHMOD    = "chmod"    // Cambiar permisos
	OPERATION_CHOWN    = "chown"    // Cambiar propietario
	OPERATION_CHGRP    = "chgrp"    // Cambiar grupo
	OPERATION_RECOVERY = "recovery" // Recuperación del sistema
	OPERATION_FORMAT   = "format"   // Formateo del sistema
)

// GetJournalSize retorna el tamaño en bytes de un Journal
func GetJournalSize() int32 {
	return int32(unsafe.Sizeof(Journal{}))
}

// NewJournal crea una nueva entrada de journal con valores por defecto
func NewJournal(count int32) *Journal {
	return &Journal{
		J_Count:   count,
		J_Content: Information{},
	}
}

// AddJournalEntry añade una nueva entrada al journal en la partición
func (fs *EXT3FileSystem) AddJournalEntry(operation string, path string, content string) error {
	// Abrir el archivo del disco
	file, err := os.OpenFile(fs.PartitionPath, os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	// Encontrar la siguiente posición de journal disponible
	currentCount := int32(0)
	journalStartOffset := fs.StartOffset + SUPERBLOCK_SIZE

	// Buscar el siguiente índice disponible o el más antiguo para sobrescribir
	// (implementación circular si se llena)
	maxJournals := fs.SuperBlock.S_inodes_count
	if maxJournals > 0 {
		// Leer el último journal para obtener el contador actual
		lastJournalOffset := journalStartOffset + (maxJournals-1)*JOURNAL_SIZE
		lastJournal := &Journal{}
		err = utilitiesP2.ReadStruct(file, int64(lastJournalOffset), lastJournal)
		if err == nil && lastJournal.J_Count > 0 {
			currentCount = lastJournal.J_Count + 1
		} else {
			currentCount = 1 // Comenzar desde 1 si no hay entradas previas o hay error
		}
	}

	// Índice circular (si se llena, sobrescribe las entradas más antiguas)
	journalIndex := (currentCount - 1) % maxJournals
	journalOffset := journalStartOffset + journalIndex*JOURNAL_SIZE

	// Crear la nueva entrada de journal
	journalEntry := NewJournal(currentCount)

	// Llenar la información
	copy(journalEntry.J_Content.I_Operation[:], operation)
	copy(journalEntry.J_Content.I_Path[:], path)
	copy(journalEntry.J_Content.I_Content[:], content)
	journalEntry.J_Content.I_Date = float64(time.Now().Unix())

	// Escribir la entrada en el disco
	err = utilitiesP2.WriteStruct(file, int64(journalOffset), journalEntry)
	if err != nil {
		return err
	}

	return nil
}

// GetJournalEntries lee todas las entradas de journal ordenadas por contador
func (fs *EXT3FileSystem) GetJournalEntries() ([]*Journal, error) {
	// Abrir el archivo del disco
	file, err := os.OpenFile(fs.PartitionPath, os.O_RDONLY, 0666)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Calcular cuántas entradas de journal pueden existir
	maxJournals := fs.SuperBlock.S_inodes_count
	journalStartOffset := fs.StartOffset + SUPERBLOCK_SIZE

	// Leer todas las entradas
	var entries []*Journal
	for i := int32(0); i < maxJournals; i++ {
		journalOffset := journalStartOffset + i*JOURNAL_SIZE
		entry := &Journal{}
		err = utilitiesP2.ReadStruct(file, int64(journalOffset), entry)
		if err != nil {
			continue
		}

		// Solo agregar entradas válidas (con contador > 0)
		if entry.J_Count > 0 {
			entries = append(entries, entry)
		}
	}

	// Ordenar las entradas por contador (más antiguo primero)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].J_Count < entries[j].J_Count
	})

	return entries, nil
}

// RecoverFromJournal intenta recuperar el sistema desde las entradas del journal
func (fs *EXT3FileSystem) RecoverFromJournal() error {
	// Obtener todas las entradas del journal
	entries, err := fs.GetJournalEntries()
	if err != nil {
		return err
	}

	// Si no hay entradas, no hay nada que recuperar
	if len(entries) == 0 {
		return nil
	}

	// Añadir entrada de inicio de recuperación al journal
	err = fs.AddJournalEntry(OPERATION_RECOVERY, "/", "Iniciando recuperación del sistema")
	if err != nil {
		return err
	}

	// Procesar cada entrada en orden cronológico
	for _, entry := range entries {
		// Extraer información de la entrada
		operation := string(entry.J_Content.I_Operation[:])
		path := string(entry.J_Content.I_Path[:])
		content := string(entry.J_Content.I_Content[:])

		// Eliminar caracteres nulos
		operation = strings.TrimRight(operation, "\x00")
		path = strings.TrimRight(path, "\x00")
		content = strings.TrimRight(content, "\x00")

		// Intentar recrear la operación según su tipo
		switch operation {
		case OPERATION_MKDIR:
			// Recrear directorio
			// Implementar la lógica específica para recrear directorios
		case OPERATION_MKFILE:
			// Recrear archivo
			// Implementar la lógica específica para recrear archivos
			// Implementar otros casos según sea necesario
		}
	}

	// Añadir entrada de finalización de recuperación al journal
	err = fs.AddJournalEntry(OPERATION_RECOVERY, "/", "Recuperación completada")
	if err != nil {
		return err
	}

	return nil
}

// ClearJournal limpia todas las entradas del journal
func (fs *EXT3FileSystem) ClearJournal() error {
	// Abrir el archivo del disco
	file, err := os.OpenFile(fs.PartitionPath, os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	// Calcular el tamaño total del área de journaling
	journalSize := fs.SuperBlock.S_inodes_count * JOURNAL_SIZE
	journalStartOffset := fs.StartOffset + SUPERBLOCK_SIZE

	// Crear un buffer de ceros para limpiar el área
	zeroBuffer := make([]byte, journalSize)

	// Posicionarse al inicio del área de journaling
	_, err = file.Seek(int64(journalStartOffset), 0)
	if err != nil {
		return err
	}

	// Escribir ceros en toda el área
	_, err = file.Write(zeroBuffer)
	if err != nil {
		return err
	}

	// Añadir una entrada inicial indicando que el journal fue limpiado
	err = fs.AddJournalEntry(OPERATION_FORMAT, "/", "Journal inicializado")
	if err != nil {
		return err
	}

	return nil
}

// GetJournalOperationName obtiene el nombre legible de una operación del journal
func GetJournalOperationName(operation string) string {
	switch operation {
	case OPERATION_MKDIR:
		return "Crear directorio"
	case OPERATION_MKFILE:
		return "Crear archivo"
	case OPERATION_RMDIR:
		return "Eliminar directorio"
	case OPERATION_RMFILE:
		return "Eliminar archivo"
	case OPERATION_EDIT:
		return "Editar archivo"
	case OPERATION_RENAME:
		return "Renombrar"
	case OPERATION_CHMOD:
		return "Cambiar permisos"
	case OPERATION_CHOWN:
		return "Cambiar propietario"
	case OPERATION_CHGRP:
		return "Cambiar grupo"
	case OPERATION_RECOVERY:
		return "Recuperación"
	case OPERATION_FORMAT:
		return "Formateo"
	default:
		return "Operación desconocida"
	}
}
