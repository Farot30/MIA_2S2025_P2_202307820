package utilitiesP2

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"unsafe"
)

// CreateDirectoryIfNotExists crea el directorio si no existe
func CreateDirectoryIfNotExists(path string) error {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}

// FileExists verifica si un archivo existe
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// ReadStruct lee una estructura desde un archivo en una posición específica
func ReadStruct(file *os.File, offset int64, data interface{}) error {
	// Moverse a la posición especificada
	if _, err := file.Seek(offset, 0); err != nil {
		return err
	}

	// Leer los datos
	return binary.Read(file, binary.LittleEndian, data)
}

// WriteStruct escribe una estructura a un archivo en una posición específica
func WriteStruct(file *os.File, offset int64, data interface{}) error {
	// Moverse a la posición especificada
	if _, err := file.Seek(offset, 0); err != nil {
		return err
	}

	// Escribir los datos
	return binary.Write(file, binary.LittleEndian, data)
}

// ReadBytes lee un número específico de bytes desde un archivo
func ReadBytes(file *os.File, offset int64, size int) ([]byte, error) {
	// Moverse a la posición especificada
	if _, err := file.Seek(offset, 0); err != nil {
		return nil, err
	}

	// Leer los bytes
	buffer := make([]byte, size)
	_, err := file.Read(buffer)
	return buffer, err
}

// WriteBytes escribe bytes a un archivo en una posición específica
func WriteBytes(file *os.File, offset int64, data []byte) error {
	// Moverse a la posición especificada
	if _, err := file.Seek(offset, 0); err != nil {
		return err
	}

	// Escribir los bytes
	_, err := file.Write(data)
	return err
}

// GetStructSize obtiene el tamaño de una estructura en bytes
func GetStructSize(data interface{}) int {
	return int(unsafe.Sizeof(data))
}
