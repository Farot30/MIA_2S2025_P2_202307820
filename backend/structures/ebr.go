package structures

import (
	"encoding/binary"
	"fmt"
	"os"
	"unsafe"
)

// EBR representa el Extended Boot Record para particiones lógicas
type EBR struct {
	PartMount byte     // Indica si la partición está montada (0 = no montada, 1 = montada)
	PartFit   byte     // Tipo de ajuste: B(Best), F(First), W(Worst)
	PartStart int32    // Byte donde inicia la partición lógica
	PartS     int32    // Tamaño total de la partición lógica en bytes
	PartNext  int32    // Byte donde inicia el siguiente EBR (-1 si no hay siguiente)
	PartName  [16]byte // Nombre de la partición lógica
}

// NewEBR crea un nuevo EBR vacío
func NewEBR() *EBR {
	ebr := &EBR{
		PartMount: 0,  // No montada por defecto
		PartFit:   0,  // Sin tipo de ajuste por defecto
		PartStart: 0,  // Sin posición inicial
		PartS:     0,  // Sin tamaño
		PartNext:  -1, // No hay siguiente EBR
	}

	// Inicializar nombre con ceros
	for i := 0; i < 16; i++ {
		ebr.PartName[i] = 0
	}

	return ebr
}

// NewEBRWithData crea un nuevo EBR con datos específicos
func NewEBRWithData(fit byte, start int32, size int32, name string, next int32) *EBR {
	ebr := &EBR{
		PartMount: 0, // No montada por defecto
		PartFit:   fit,
		PartStart: start,
		PartS:     size,
		PartNext:  next,
	}

	// Inicializar nombre con ceros
	for i := 0; i < 16; i++ {
		ebr.PartName[i] = 0
	}

	// Copiar el nombre (máximo 15 caracteres para dejar espacio para null terminator)
	nameBytes := []byte(name)
	maxLen := 15
	if len(nameBytes) < maxLen {
		maxLen = len(nameBytes)
	}
	copy(ebr.PartName[:], nameBytes[:maxLen])

	return ebr
}

// ToBytes convierte el EBR a bytes para escribir al disco
func (ebr *EBR) ToBytes() []byte {
	size := int(unsafe.Sizeof(*ebr))
	bytes := make([]byte, size)

	offset := 0

	// PartMount
	bytes[offset] = ebr.PartMount
	offset += 1

	// PartFit
	bytes[offset] = ebr.PartFit
	offset += 1

	// PartStart
	binary.LittleEndian.PutUint32(bytes[offset:], uint32(ebr.PartStart))
	offset += 4

	// PartS
	binary.LittleEndian.PutUint32(bytes[offset:], uint32(ebr.PartS))
	offset += 4

	// PartNext
	binary.LittleEndian.PutUint32(bytes[offset:], uint32(ebr.PartNext))
	offset += 4

	// PartName
	copy(bytes[offset:], ebr.PartName[:])

	return bytes
}

// FromBytes lee un EBR desde bytes
func (ebr *EBR) FromBytes(data []byte) error {
	if len(data) < int(unsafe.Sizeof(*ebr)) {
		return fmt.Errorf("datos insuficientes para leer EBR")
	}

	offset := 0

	// PartMount
	ebr.PartMount = data[offset]
	offset += 1

	// PartFit
	ebr.PartFit = data[offset]
	offset += 1

	// PartStart
	ebr.PartStart = int32(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4

	// PartS
	ebr.PartS = int32(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4

	// PartNext
	ebr.PartNext = int32(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4

	// PartName
	copy(ebr.PartName[:], data[offset:offset+16])

	return nil
}

// WriteEBR escribe un EBR al disco en la posición especificada
func WriteEBR(file *os.File, ebr *EBR, position int32) error {
	// Posicionarse en la ubicación correcta
	if _, err := file.Seek(int64(position), 0); err != nil {
		return fmt.Errorf("error posicionándose en el disco: %v", err)
	}

	// Escribir el EBR
	ebrBytes := ebr.ToBytes()
	if _, err := file.Write(ebrBytes); err != nil {
		return fmt.Errorf("error escribiendo EBR: %v", err)
	}

	return nil
}

// ReadEBR lee un EBR desde el disco en la posición especificada
func ReadEBR(file *os.File, position int32) (*EBR, error) {
	// Posicionarse en la ubicación correcta
	if _, err := file.Seek(int64(position), 0); err != nil {
		return nil, fmt.Errorf("error posicionándose en el disco: %v", err)
	}

	// Leer los datos del EBR
	ebrSize := int(unsafe.Sizeof(EBR{}))
	data := make([]byte, ebrSize)
	if _, err := file.Read(data); err != nil {
		return nil, fmt.Errorf("error leyendo EBR: %v", err)
	}

	// Crear y llenar el EBR
	ebr := NewEBR()
	if err := ebr.FromBytes(data); err != nil {
		return nil, err
	}

	return ebr, nil
}

// GetSize retorna el tamaño del EBR en bytes
func (ebr *EBR) GetSize() int {
	return int(unsafe.Sizeof(*ebr))
}

// IsEmpty verifica si el EBR está vacío (no contiene una partición lógica)
func (ebr *EBR) IsEmpty() bool {
	return ebr.PartS == 0 && ebr.PartStart == 0
}

// SetName establece el nombre de la partición lógica
func (ebr *EBR) SetName(name string) {
	// Limpiar el nombre actual
	for i := 0; i < 16; i++ {
		ebr.PartName[i] = 0
	}

	// Copiar el nuevo nombre (máximo 15 caracteres)
	nameBytes := []byte(name)
	maxLen := 15
	if len(nameBytes) < maxLen {
		maxLen = len(nameBytes)
	}
	copy(ebr.PartName[:], nameBytes[:maxLen])
}

// GetName retorna el nombre de la partición lógica como string
func (ebr *EBR) GetName() string {
	// Encontrar el final del string (primer byte 0)
	nameLen := 0
	for i := 0; i < 16; i++ {
		if ebr.PartName[i] == 0 {
			break
		}
		nameLen++
	}
	return string(ebr.PartName[:nameLen])
}

// HasNext verifica si hay un siguiente EBR en la cadena
func (ebr *EBR) HasNext() bool {
	return ebr.PartNext != -1 && ebr.PartNext > 0
}
