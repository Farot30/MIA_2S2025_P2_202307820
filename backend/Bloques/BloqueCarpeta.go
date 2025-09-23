package BloquesP2

import (
	"fmt"
	"strings"
	"unsafe"
)

type B_content struct {
	B_name  [12]byte // Nombre de la carpeta o archivo (12 caracteres)
	B_inodo int32    // Apuntador hacia un inodo asociado al archivo o carpeta
}

type BloqueCarpeta struct {
	B_content [4]B_content // Array con el contenido de la carpeta (4 entradas)
}

// NewB_content crea una nueva entrada de carpeta
func NewB_content(name string, inodeNum int32) B_content {
	var entry B_content

	// Copiar el nombre (máximo 12 bytes)
	nameBytes := []byte(name)
	if len(nameBytes) > 12 {
		nameBytes = nameBytes[:12]
	}
	copy(entry.B_name[:], nameBytes)

	entry.B_inodo = inodeNum
	return entry
}

// GetName extrae el nombre de la entrada
func (bc *B_content) GetName() string {
	return strings.TrimRight(string(bc.B_name[:]), "\x00")
}

// SetName establece el nombre de la entrada
func (bc *B_content) SetName(name string) {
	// Limpiar el array
	for i := range bc.B_name {
		bc.B_name[i] = 0
	}

	// Copiar el nuevo nombre (máximo 12 bytes)
	nameBytes := []byte(name)
	if len(nameBytes) > 12 {
		nameBytes = nameBytes[:12]
	}
	copy(bc.B_name[:], nameBytes)
}

// GetInodeNumber retorna el número de inodo
func (bc *B_content) GetInodeNumber() int32 {
	return bc.B_inodo
}

// SetInodeNumber establece el número de inodo
func (bc *B_content) SetInodeNumber(inodeNum int32) {
	bc.B_inodo = inodeNum
}

// IsEmpty verifica si la entrada está vacía
func (bc *B_content) IsEmpty() bool {
	return bc.B_inodo == -1
}

// Clear limpia la entrada
func (bc *B_content) Clear() {
	bc.B_inodo = -1 // Marcar como libre
	for i := range bc.B_name {
		bc.B_name[i] = 0
	}
}

// NewBloqueCarpeta crea un nuevo bloque de carpeta vacío
func NewBloqueCarpeta() *BloqueCarpeta {
	bc := &BloqueCarpeta{}
	// Inicializar todas las entradas como libres
	for i := range bc.B_content {
		bc.B_content[i].B_inodo = -1
	}
	return bc
}

// IsEntryEmpty verifica si una entrada específica está vacía
func (bc *BloqueCarpeta) IsEntryEmpty(index int) bool {
	if index < 0 || index >= 4 {
		return false
	}
	// Solo -1 significa libre; inode 0 es válido (p. ej. root)
	return bc.B_content[index].B_inodo == -1
}

// SetEntry establece una entrada en el índice especificado
func (bc *BloqueCarpeta) SetEntry(index int, name string, inodeNum int32) bool {
	if index < 0 || index >= 4 {
		return false
	}

	// Limpiar el array de nombre
	for i := range bc.B_content[index].B_name {
		bc.B_content[index].B_name[i] = 0
	}

	// Copiar el nombre (máximo 11 caracteres + terminador nulo)
	nameBytes := []byte(name)
	copyLen := len(nameBytes)
	if copyLen > 11 {
		copyLen = 11
	}

	copy(bc.B_content[index].B_name[:copyLen], nameBytes[:copyLen])
	bc.B_content[index].B_inodo = inodeNum

	return true
}

// GetEntry obtiene una entrada específica por índice
func (bc *BloqueCarpeta) GetEntry(index int) (string, int32, bool) {
	if index < 0 || index >= 4 {
		return "", -1, false
	}

	if bc.IsEntryEmpty(index) {
		return "", -1, false
	}

	name := strings.TrimRight(string(bc.B_content[index].B_name[:]), "\x00")
	return name, bc.B_content[index].B_inodo, true
}

// AddEntry agrega una entrada al bloque de carpeta
func (bc *BloqueCarpeta) AddEntry(name string, inodeNum int32) bool {
	for i := range bc.B_content {
		if bc.B_content[i].IsEmpty() {
			bc.B_content[i] = NewB_content(name, inodeNum)
			return true
		}
	}
	return false // Bloque lleno
}

// FindEntry busca una entrada por nombre
func (bc *BloqueCarpeta) FindEntry(name string) (int32, bool) {
	for _, entry := range bc.B_content {
		if !entry.IsEmpty() && entry.GetName() == name {
			return entry.GetInodeNumber(), true
		}
	}
	return -1, false
}

// FindEntryByName busca una entrada por nombre y retorna su índice
func (bc *BloqueCarpeta) FindEntryByName(name string) (int, int32, bool) {
	for i := 0; i < 4; i++ {
		if !bc.IsEntryEmpty(i) {
			entryName := strings.TrimRight(string(bc.B_content[i].B_name[:]), "\x00")
			if entryName == name {
				return i, bc.B_content[i].B_inodo, true
			}
		}
	}
	return -1, -1, false
}

// GetEntries retorna todas las entradas no vacías
func (bc *BloqueCarpeta) GetEntries() []B_content {
	var entries []B_content
	for _, entry := range bc.B_content {
		if !entry.IsEmpty() {
			entries = append(entries, entry)
		}
	}
	return entries
}

// CountEntries cuenta las entradas no vacías
func (bc *BloqueCarpeta) CountEntries() int {
	count := 0
	for i := 0; i < 4; i++ {
		if !bc.IsEntryEmpty(i) {
			count++
		}
	}
	return count
}

// IsFull verifica si el bloque está lleno
func (bc *BloqueCarpeta) IsFull() bool {
	return bc.CountEntries() >= 4
}

// HasSpace verifica si hay espacio para más entradas
func (bc *BloqueCarpeta) HasSpace() bool {
	return bc.CountEntries() < 4
}

// GetFirstFreeIndex retorna el primer índice libre
func (bc *BloqueCarpeta) GetFirstFreeIndex() int {
	for i := 0; i < 4; i++ {
		if bc.IsEntryEmpty(i) {
			return i
		}
	}
	return -1 // No hay espacios libres
}

// Clear limpia todas las entradas del bloque
func (bc *BloqueCarpeta) Clear() {
	for i := 0; i < 4; i++ {
		bc.B_content[i].Clear()
	}
}

// GetSize retorna el tamaño del bloque en bytes (4 * (12 + 4) = 64 bytes)
func (bc *BloqueCarpeta) GetSize() int32 {
	return int32(unsafe.Sizeof(*bc))
}

// ListEntries retorna una lista legible de todas las entradas (para debugging)
func (bc *BloqueCarpeta) ListEntries() []string {
	var entries []string
	for i := 0; i < 4; i++ {
		if !bc.IsEntryEmpty(i) {
			name := strings.TrimRight(string(bc.B_content[i].B_name[:]), "\x00")
			entries = append(entries, fmt.Sprintf("%d: %s -> inode %d", i, name, bc.B_content[i].B_inodo))
		} else {
			entries = append(entries, fmt.Sprintf("%d: [VACÍO]", i))
		}
	}
	return entries
}

func (bc *BloqueCarpeta) String() string {
	return fmt.Sprintf("BloqueCarpeta{entries: %d/4, size: %d bytes}", bc.CountEntries(), bc.GetSize())
}
