package BloquesP2

import (
	"bytes"
)

// GetBytesContent obtiene el contenido del bloque como un array de bytes
func (ba *BloqueArchivo) GetBytesContent() [64]byte {
	return ba.B_content
}

// SetBytesContent establece el contenido del bloque con un array de bytes
func (ba *BloqueArchivo) SetBytesContent(content [64]byte) {
	ba.B_content = content
}

// GetBytesContentByIndex obtiene un fragmento de bytes por índice
func (ba *BloqueArchivo) GetBytesContentByIndex(index int, size int) []byte {
	if index < 0 || index >= 64 || index+size > 64 {
		return nil
	}

	result := make([]byte, size)
	copy(result, ba.B_content[index:index+size])
	return result
}

// SetBytesContentByIndex establece un fragmento de bytes por índice
func (ba *BloqueArchivo) SetBytesContentByIndex(index int, content []byte) bool {
	if index < 0 || index >= 64 || index+len(content) > 64 {
		return false
	}

	copy(ba.B_content[index:index+len(content)], content)
	return true
}

// ContainsOnlyNullBytes verifica si solo hay bytes nulos desde un índice particular
func (ba *BloqueArchivo) ContainsOnlyNullBytes(startIndex int) bool {
	if startIndex < 0 || startIndex >= 64 {
		return true
	}

	for i := startIndex; i < 64; i++ {
		if ba.B_content[i] != 0 {
			return false
		}
	}
	return true
}

// TrimNullBytes elimina los bytes nulos al final del contenido
func (ba *BloqueArchivo) TrimNullBytes() []byte {
	return bytes.TrimRight(ba.B_content[:], "\x00")
}
