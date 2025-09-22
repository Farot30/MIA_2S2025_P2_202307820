package estructuraarchivocarpeta

import (
	"time"
	"unsafe"
)

// SuperBloque representa la estructura del superbloque del sistema de archivos EXT2
type SuperBloque struct {
	S_filesystem_type   int32 // Guarda el número que identifica el sistema de archivos utilizado (2 para EXT2)
	S_inodes_count      int32 // Guarda el número total de inodos
	S_blocks_count      int32 // Guarda el número total de bloques
	S_free_blocks_count int32 // Contiene el número de bloques libres
	S_free_inodes_count int32 // Contiene el número de inodos libres
	S_mtime             int64 // Última fecha en el que el sistema fue montado (timestamp Unix)
	S_umtime            int64 // Última fecha en que el sistema fue desmontado (timestamp Unix)
	S_mnt_count         int32 // Indica cuantas veces se ha montado el sistema
	S_magic             int32 // Valor que identifica al sistema de archivos (0xEF53)
	S_inode_s           int32 // Tamaño del inodo
	S_block_s           int32 // Tamaño del bloque
	S_firts_ino         int32 // Primer inodo libre (dirección del inodo)
	S_first_blo         int32 // Primer bloque libre (dirección del bloque)
	S_bm_inode_start    int32 // Guardará el inicio del bitmap de inodos
	S_bm_block_start    int32 // Guardará el inicio del bitmap de bloques
	S_inode_start       int32 // Guardará el inicio de la tabla de inodos
	S_block_start       int32 // Guardará el inicio de la tabla de bloques
}

// Constantes para el sistema de archivos EXT2
const (
	EXT2_FILESYSTEM_TYPE = 2      // Identificador del sistema de archivos EXT2
	EXT2_MAGIC           = 0xEF53 // Número mágico del sistema EXT2
	DEFAULT_INODE_SIZE   = 128    // Tamaño por defecto del inodo
	DEFAULT_BLOCK_SIZE   = 64     // Tamaño por defecto del bloque
)

// NewSuperBloque crea un nuevo superbloque inicializado con valores por defecto
func NewSuperBloque(partitionSize int32) *SuperBloque {
	now := GetCurrentTimestamp()

	// Calcular el número de bloques basado en el tamaño de la partición
	blocksCount := partitionSize / DEFAULT_BLOCK_SIZE
	inodesCount := blocksCount / 3 // Aproximadamente 1/3 de los bloques para inodos

	// Calcular posiciones de las estructuras
	superBlockSize := int32(unsafe.Sizeof(SuperBloque{}))
	bitmapInodeStart := superBlockSize
	bitmapBlockStart := bitmapInodeStart + (inodesCount / 8) // 1 bit por inodo
	inodeTableStart := bitmapBlockStart + (blocksCount / 8)  // 1 bit por bloque
	blockTableStart := inodeTableStart + (inodesCount * DEFAULT_INODE_SIZE)

	return &SuperBloque{
		S_filesystem_type:   EXT2_FILESYSTEM_TYPE,
		S_inodes_count:      inodesCount,
		S_blocks_count:      blocksCount,
		S_free_blocks_count: blocksCount - 1, // -1 por el superbloque
		S_free_inodes_count: inodesCount - 1, // -1 por el inodo raíz
		S_mtime:             now,
		S_umtime:            now,
		S_mnt_count:         0,
		S_magic:             EXT2_MAGIC,
		S_inode_s:           DEFAULT_INODE_SIZE,
		S_block_s:           DEFAULT_BLOCK_SIZE,
		S_firts_ino:         2, // El primer inodo libre (1 es el raíz)
		S_first_blo:         1, // El primer bloque libre (0 es el superbloque)
		S_bm_inode_start:    bitmapInodeStart,
		S_bm_block_start:    bitmapBlockStart,
		S_inode_start:       inodeTableStart,
		S_block_start:       blockTableStart,
	}
}

// GetCurrentTimestamp retorna el timestamp Unix actual
func GetCurrentTimestamp() int64 {
	return time.Now().Unix()
}

// Mount marca el superbloque como montado
func (sb *SuperBloque) Mount() {
	sb.S_mtime = GetCurrentTimestamp()
	sb.S_mnt_count++
}

// Unmount marca el superbloque como desmontadoj
func (sb *SuperBloque) Unmount() {
	sb.S_umtime = GetCurrentTimestamp()
}

// AllocateInode reserva un inodo y actualiza los contadores
func (sb *SuperBloque) AllocateInode() int32 {
	if sb.S_free_inodes_count <= 0 {
		return -1 // No hay inodos disponibles
	}

	currentInode := sb.S_firts_ino
	sb.S_firts_ino++
	sb.S_free_inodes_count--

	return currentInode
}

// FreeInode libera un inodo y actualiza los contadores
func (sb *SuperBloque) FreeInode(inodeNum int32) {
	if inodeNum < sb.S_firts_ino {
		sb.S_firts_ino = inodeNum
	}
	sb.S_free_inodes_count++
}

// AllocateBlock reserva un bloque y actualiza los contadores
func (sb *SuperBloque) AllocateBlock() int32 {
	if sb.S_free_blocks_count <= 0 {
		return -1 // No hay bloques disponibles
	}

	currentBlock := sb.S_first_blo
	sb.S_first_blo++
	sb.S_free_blocks_count--

	return currentBlock
}

// FreeBlock libera un bloque y actualiza los contadores
func (sb *SuperBloque) FreeBlock(blockNum int32) {
	if blockNum < sb.S_first_blo {
		sb.S_first_blo = blockNum
	}
	sb.S_free_blocks_count++
}

// IsValid verifica si el superbloque es válido
func (sb *SuperBloque) IsValid() bool {
	return sb.S_magic == EXT2_MAGIC &&
		sb.S_filesystem_type == EXT2_FILESYSTEM_TYPE
}

// GetSize retorna el tamaño del superbloque en bytes
func (sb *SuperBloque) GetSize() int32 {
	return int32(unsafe.Sizeof(*sb))
}
