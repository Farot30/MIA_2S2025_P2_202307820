package bloques

import (
	"strings"
	"unsafe"
)

// BloqueArchivo estructura que guarda el contenido de un archivo
type BloqueArchivo struct {
	B_content [64]byte // Array con el contenido del archivo (64 caracteres)
}

// NewBloqueArchivo crea un nuevo bloque de archivo vacío
func NewBloqueArchivo() *BloqueArchivo {
	return &BloqueArchivo{}
}

// SetContent establece el contenido del archivo
func (ba *BloqueArchivo) SetContent(content string) {
	// Limpiar el array
	for i := range ba.B_content {
		ba.B_content[i] = 0
	}

	// Copiar el contenido (máximo 64 bytes)
	contentBytes := []byte(content)
	if len(contentBytes) > 64 {
		contentBytes = contentBytes[:64]
	}
	copy(ba.B_content[:], contentBytes)
}

// GetContent obtiene el contenido del archivo como string
func (ba *BloqueArchivo) GetContent() string {
	return strings.TrimRight(string(ba.B_content[:]), "\x00")
}

// AppendContent agrega contenido al final del bloque
func (ba *BloqueArchivo) AppendContent(content string) bool {
	currentContent := ba.GetContent()
	newContent := currentContent + content

	if len(newContent) > 64 {
		return false // No cabe más contenido
	}

	ba.SetContent(newContent)
	return true
}

// Clear limpia el contenido del bloque
func (ba *BloqueArchivo) Clear() {
	for i := range ba.B_content {
		ba.B_content[i] = 0
	}
}

// IsEmpty verifica si el bloque está vacío
func (ba *BloqueArchivo) IsEmpty() bool {
	return ba.B_content[0] == 0
}

// GetAvailableSpace retorna el espacio disponible en bytes
func (ba *BloqueArchivo) GetAvailableSpace() int {
	currentLength := len(ba.GetContent())
	return 64 - currentLength
}

// GetSize retorna el tamaño del bloque en bytes (64 bytes)
func (ba *BloqueArchivo) GetSize() int32 {
	return int32(unsafe.Sizeof(*ba))
}
