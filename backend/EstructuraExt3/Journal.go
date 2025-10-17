package EstructuraExt3P2

import (
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unsafe"
)

type Information struct {
	I_Operation [10]byte // Tipo de operación (10 caracteres)
	I_Path      [32]byte // Ruta del archivo o carpeta (40 caracteres)
	I_Date      float64  // Fecha y hora de la operación (timestamp float64)
	I_Content   [64]byte // Contenido adicional (64 caracteres)
}

type Journal struct {
	J_Count   int32       // Lleva el conteo del journal que es
	J_Content Information // Estructura de información del journal
}

const (
	// Tipos de operaciones para el journal
	OPERATION_MKDIR   = "mkdir"
	OPERATION_MKFILE  = "mkfile"
	OPERATION_RMDIR   = "rmdir"
	OPERATION_RMFILE  = "rmfile"
	OPERATION_RMGRP   = "rmgrp"
	OPERATION_RMUSR   = "rmusr"
	OPERATION_EDIT    = "edit"
	OPERATION_RENAME  = "rename"
	OPERATION_CHMOD   = "chmod"
	OPERATION_CHOWN   = "chown"
	OPERATION_CHGRP   = "chgrp"
	OPERATION_MKGRP   = "mkgrp"
	OPERATION_MKUSR   = "mkusr"
	OPERATION_LOGIN   = "login"
	OPERATION_LOGOUT  = "logout"
	OPERATION_FORMAT  = "format"
	OPERATION_READ    = "read"
	OPERATION_FIND    = "find"
	OPERATION_COPY    = "copy"
	OPERATION_MOVE    = "move"
	OPERATION_MOUNT   = "mount"
	OPERATION_UNMOUNT = "unmount"
	OPERATION_LOSS    = "loss"
	OPERATION_CAT     = "cat"
)

// Variable global para el tamaño del journal calculado dinámicamente
var JOURNAL_SIZE = int32(unsafe.Sizeof(Journal{}))

// GetJournalSize retorna el tamaño en bytes de un Journal
func GetJournalSize() int32 {
	return JOURNAL_SIZE
}

// isValidOperation verifica si una operación es válida
func isValidOperation(operation string) bool {
	validOps := map[string]bool{
		OPERATION_MKDIR:   true,
		OPERATION_MKFILE:  true,
		OPERATION_RMDIR:   true,
		OPERATION_RMFILE:  true,
		OPERATION_RMGRP:   true,
		OPERATION_RMUSR:   true,
		OPERATION_EDIT:    true,
		OPERATION_RENAME:  true,
		OPERATION_CHMOD:   true,
		OPERATION_CHOWN:   true,
		OPERATION_CHGRP:   true,
		OPERATION_MKGRP:   true,
		OPERATION_MKUSR:   true,
		OPERATION_LOGIN:   true,
		OPERATION_LOGOUT:  true,
		OPERATION_FORMAT:  true,
		OPERATION_READ:    true,
		OPERATION_FIND:    true,
		OPERATION_COPY:    true,
		OPERATION_MOVE:    true,
		OPERATION_MOUNT:   true,
		OPERATION_UNMOUNT: true,
		OPERATION_LOSS:    true,
		OPERATION_CAT:     true,
	}
	return validOps[operation]
}

// NewJournal crea una nueva entrada de journal con valores por defecto
func NewJournal(count int32) *Journal {
	return &Journal{
		J_Count:   count,
		J_Content: Information{},
	}
}

// AddJournalEntry añade una nueva entrada al journal con validación mejorada
func (fs *EXT3FileSystem) AddJournalEntry(operation string, path string, content string) error {
	// Validaciones iniciales
	if fs == nil || fs.PartitionPath == "" {
		return fmt.Errorf("sistema de archivos no inicializado")
	}

	// Limpiar y validar strings
	operation = strings.TrimSpace(operation)
	path = strings.TrimSpace(path)
	content = strings.TrimSpace(content)

	// Validar que la operación sea válida
	if !isValidOperation(operation) {
		fmt.Printf("WARNING: Operación inválida '%s' - no se registrará en journal\n", operation)
		return nil // No fallar, solo advertir
	}

	// Truncar strings si son muy largos
	if len(operation) > 9 {
		operation = operation[:9]
	}
	if len(path) > 31 {
		path = path[:31]
	}
	if len(content) > 63 {
		content = content[:63]
	}

	// Abrir el archivo del disco
	file, err := os.OpenFile(fs.PartitionPath, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo partición %s: %v", fs.PartitionPath, err)
	}
	defer file.Close()

	// Obtener el próximo contador secuencial
	nextCount, err := fs.getNextJournalCount(file)
	if err != nil {
		return fmt.Errorf("error obteniendo siguiente contador: %v", err)
	}

	// Calcular la posición donde escribir (circular)
	maxJournals := fs.SuperBlock.S_inodes_count
	if maxJournals <= 0 {
		return fmt.Errorf("número de inodos inválido: %d", maxJournals)
	}

	journalIndex := (nextCount - 1) % maxJournals
	journalStartOffset := fs.StartOffset + SUPERBLOCK_SIZE
	journalOffset := journalStartOffset + journalIndex*JOURNAL_SIZE

	// Crear la nueva entrada de journal
	journalEntry := NewJournal(nextCount)

	// Limpiar los arrays primero
	for i := range journalEntry.J_Content.I_Operation {
		journalEntry.J_Content.I_Operation[i] = 0
	}
	for i := range journalEntry.J_Content.I_Path {
		journalEntry.J_Content.I_Path[i] = 0
	}
	for i := range journalEntry.J_Content.I_Content {
		journalEntry.J_Content.I_Content[i] = 0
	}

	// Llenar la información
	copy(journalEntry.J_Content.I_Operation[:], []byte(operation))
	copy(journalEntry.J_Content.I_Path[:], []byte(path))
	copy(journalEntry.J_Content.I_Content[:], []byte(content))
	journalEntry.J_Content.I_Date = float64(time.Now().Unix())

	// Escribir la entrada en el disco
	err = utilitiesP2.WriteStruct(file, int64(journalOffset), journalEntry)
	if err != nil {
		return fmt.Errorf("error escribiendo journal en offset %d: %v", journalOffset, err)
	}

	// Forzar sincronización con el disco
	err = file.Sync()
	if err != nil {
		return fmt.Errorf("error sincronizando journal: %v", err)
	}

	return nil
}

// getNextJournalCount obtiene el siguiente contador de journal de manera segura
func (fs *EXT3FileSystem) getNextJournalCount(file *os.File) (int32, error) {
	maxJournals := fs.SuperBlock.S_inodes_count
	journalStartOffset := fs.StartOffset + SUPERBLOCK_SIZE
	maxCount := int32(0)

	// Escanear todas las entradas para encontrar el contador más alto
	for i := int32(0); i < maxJournals; i++ {
		journalOffset := journalStartOffset + i*JOURNAL_SIZE
		entry := &Journal{}

		err := utilitiesP2.ReadStruct(file, int64(journalOffset), entry)
		if err != nil {
			continue // Si hay error leyendo, continuar
		}

		// Validar que el contador sea razonable
		if entry.J_Count > maxCount && entry.J_Count < 1000000 {
			maxCount = entry.J_Count
		}
	}

	return maxCount + 1, nil
}

// GetJournalEntries lee todas las entradas válidas del journal
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

		// Solo agregar entradas válidas
		if entry.J_Count > 0 && entry.J_Count < 1000000 {
			operation := strings.TrimRight(string(entry.J_Content.I_Operation[:]), "\x00")
			operation = strings.TrimSpace(operation)

			// Solo agregar operaciones válidas y no vacías
			if operation != "" && isValidOperation(operation) {
				// Validar timestamp
				if entry.J_Content.I_Date > 0 && entry.J_Content.I_Date < 4000000000 {
					entries = append(entries, entry)
				}
			}
		}
	}

	// Ordenar las entradas por contador (más reciente primero)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].J_Count > entries[j].J_Count
	})

	return entries, nil
}

// LogOperation es un helper para registrar operaciones con manejo de errores
func (fs *EXT3FileSystem) LogOperation(operation, path, details string) {
	if fs == nil {
		return
	}

	err := fs.AddJournalEntry(operation, path, details)
	if err != nil {
		fmt.Printf("WARNING: Error registrando operación '%s' en journal: %v\n", operation, err)
	}
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

	// Sincronizar
	err = file.Sync()
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

	if err != nil {
		return err
	}

	// Procesar cada entrada en orden cronológico
	for _, entry := range entries {
		// Extraer información de la entrada
		operation := strings.TrimRight(string(entry.J_Content.I_Operation[:]), "\x00")
		path := strings.TrimRight(string(entry.J_Content.I_Path[:]), "\x00")
		content := strings.TrimRight(string(entry.J_Content.I_Content[:]), "\x00")

		// Limpiar espacios
		operation = strings.TrimSpace(operation)
		path = strings.TrimSpace(path)
		content = strings.TrimSpace(content)

		// Intentar recrear la operación según su tipo
		switch operation {
		case OPERATION_MKDIR:
			// Implementar la lógica específica para recrear directorios
		case OPERATION_MKFILE:
			// Implementar la lógica específica para recrear archivos
		}
	}

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
	case OPERATION_FORMAT:
		return "Formateo"
	case OPERATION_LOGIN:
		return "Inicio de sesión"
	case OPERATION_LOGOUT:
		return "Cierre de sesión"
	case OPERATION_MKGRP:
		return "Crear grupo"
	case OPERATION_MKUSR:
		return "Crear usuario"
	case OPERATION_RMGRP:
		return "Eliminar grupo"
	case OPERATION_RMUSR:
		return "Eliminar usuario"
	case OPERATION_READ:
		return "Leer archivo"
	case OPERATION_FIND:
		return "Buscar archivo"
	case OPERATION_COPY:
		return "Copiar archivo"
	case OPERATION_MOVE:
		return "Mover archivo"
	case OPERATION_MOUNT:
		return "Montar partición"
	case OPERATION_UNMOUNT:
		return "Desmontar partición"
	case OPERATION_CAT:
		return "Leer contenido"
	default:
		return "Operación desconocida"
	}
}
