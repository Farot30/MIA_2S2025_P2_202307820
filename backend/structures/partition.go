package structuresP2

import (
	"encoding/binary"
	"unsafe"
)

// Partition representa una partición
type Partition struct {
	PartStatus      byte     // Indica si la partición está montada o no
	PartType        byte     // Tipo de partición: P(Primaria), E(Extendida)
	PartFit         byte     // Tipo de ajuste: B(Best), F(First), W(Worst)
	PartStart       int32    // Byte donde inicia la partición
	PartS           int32    // Tamaño total de la partición en bytes
	PartName        [16]byte // Nombre de la partición
	PartCorrelative int32    // Correlativo de la partición (-1 hasta montaje)
	PartId          [4]byte  // ID de la partición generado al montar
}

// ToBytes convierte la partición a bytes
func (p *Partition) ToBytes() []byte {
	size := int(unsafe.Sizeof(*p))
	bytes := make([]byte, size)

	offset := 0

	// PartStatus
	bytes[offset] = p.PartStatus
	offset += 1

	// PartType
	bytes[offset] = p.PartType
	offset += 1

	// PartFit
	bytes[offset] = p.PartFit
	offset += 1

	// PartStart
	binary.LittleEndian.PutUint32(bytes[offset:], uint32(p.PartStart))
	offset += 4

	// PartS
	binary.LittleEndian.PutUint32(bytes[offset:], uint32(p.PartS))
	offset += 4

	// PartName
	copy(bytes[offset:], p.PartName[:])
	offset += 16

	// PartCorrelative
	binary.LittleEndian.PutUint32(bytes[offset:], uint32(p.PartCorrelative))
	offset += 4

	// PartId
	copy(bytes[offset:], p.PartId[:])

	return bytes
}
