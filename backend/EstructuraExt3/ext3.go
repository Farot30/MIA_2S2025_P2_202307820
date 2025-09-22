package EstructuraExt3

import (
	estructuraarchivocarpeta "MIA_2S2025_P1_202307820/estructuraArchivoCarpeta"
	bloques "MIA_2S2025_P2_202307820/Bloques"
	"unsafe"
)

type EXT3FileSystem struct {
	SuperBlock    *estructuraarchivocarpeta.SuperBloque
	PartitionPath string
	PartitionSize int32
	StartOffset   int32
}

const (
	SUPERBLOCK_SIZE = int32(unsafe.Sizeof(estructuraarchivocarpeta.SuperBloque{}))
	INODE_SIZE      = int32(unsafe.Sizeof(estructuraarchivocarpeta.Inodo{}))
	BLOCK_SIZE      = int32(unsafe.Sizeof(bloques.BloqueArchivo{}))
)
