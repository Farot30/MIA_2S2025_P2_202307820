package structuresP2

import (
	"encoding/binary"
	"math/rand"
	"time"
	"unsafe"
)

// MBR representa el Master Boot Record
type MBR struct {
	MbrTamano        int32        // Tamaño total del disco en bytes
	MbrFechaCreacion [19]byte     // Fecha y hora de creación del disco
	MbrDskSignature  int32        // Número random que identifica el disco
	DskFit           byte         // Tipo de ajuste: B(Best), F(First), W(Worst)
	MbrPartitions    [4]Partition // Array de 4 particiones
}

// NewMBR crea un nuevo MBR
func NewMBR(size int32, fit byte) *MBR {
	mbr := &MBR{
		MbrTamano:       size,
		MbrDskSignature: rand.Int31(),
		DskFit:          fit,
	}

	// Establecer fecha de creación
	now := time.Now().Format("2006-01-02 15:04:05")
	copy(mbr.MbrFechaCreacion[:], now)

	// Inicializar particiones vacías con ceros binarios
	for i := 0; i < 4; i++ {
		mbr.MbrPartitions[i] = Partition{
			PartStatus:      0, // 0x00 en lugar de '0' (0x30)
			PartType:        0, // 0x00 en lugar de '0' (0x30)
			PartFit:         0, // 0x00 en lugar de '0' (0x30)
			PartStart:       0, // 0x00000000 en lugar de -1 (0xFFFFFFFF)
			PartS:           0,
			PartCorrelative: 0, // 0x00000000 en lugar de -1 (0xFFFFFFFF)
		}
		// Inicializar PartName y PartId con ceros
		for j := 0; j < 16; j++ {
			mbr.MbrPartitions[i].PartName[j] = 0
		}
		for j := 0; j < 4; j++ {
			mbr.MbrPartitions[i].PartId[j] = 0
		}
	}

	return mbr
}

// ToBytes convierte el MBR a bytes para escribir al disco
func (mbr *MBR) ToBytes() []byte {
	size := int(unsafe.Sizeof(*mbr))
	bytes := make([]byte, size)

	offset := 0

	// MbrTamano
	binary.LittleEndian.PutUint32(bytes[offset:], uint32(mbr.MbrTamano))
	offset += 4

	// MbrFechaCreacion
	copy(bytes[offset:], mbr.MbrFechaCreacion[:])
	offset += 19

	// MbrDskSignature
	binary.LittleEndian.PutUint32(bytes[offset:], uint32(mbr.MbrDskSignature))
	offset += 4

	// DskFit
	bytes[offset] = mbr.DskFit
	offset += 1

	// MbrPartitions
	for i := 0; i < 4; i++ {
		partBytes := mbr.MbrPartitions[i].ToBytes()
		copy(bytes[offset:], partBytes)
		offset += len(partBytes)
	}

	return bytes
}

// GetSize retorna el tamaño del MBR en bytes
func (mbr *MBR) GetSize() int {
	return int(unsafe.Sizeof(*mbr))
}
