package EstructuraarchivocarpetaP2

import "fmt"

type Inodo struct {
	Iuid   int32     // UID del usuario propietario del archivo o carpeta
	Igid   int32     // GID del grupo al que pertenece el archivo o carpeta
	Is     int32     // Tamaño del archivo en bytes
	Iatime int64     // Última fecha en que se leyó el inodo sin modificarlo
	Ictime int64     // Fecha en la que se creó el inodo
	Imtime int64     // Última fecha en la que se modifica el inodo
	Iblock [16]int32 // Array de bloques: [0-11] directos, [12] simple indirecto, [13] doble indirecto, [14] triple indirecto, [15] reservado
	Itype  byte      // CAMBIO: Indica si es archivo (1) o carpeta (0)
	Iperm  [3]byte   // AMBIO: Permisos UGO (User Group Other) en forma octal
}

// SetRootValues configura el inodo como directorio raíz
func (i *Inodo) SetRootValues() {
	now := GetCurrentTimestamp()
	i.Iuid = 0
	i.Igid = 0
	i.Is = 64
	i.Iatime = now
	i.Ictime = now
	i.Imtime = now
	i.Itype = 0                // CAMBIO: 0 = Carpeta (era '0')
	i.Iperm = [3]byte{7, 5, 5} // CAMBIO: byte en lugar de int8
	i.Iblock[0] = 0
	for j := 1; j < 16; j++ {
		i.Iblock[j] = -1
	}
}

// SetFileValues configura el inodo para un archivo
func (i *Inodo) SetFileValues(uid, gid int32, size int32, fileType byte, perm [3]byte) {
	now := GetCurrentTimestamp()
	i.Iuid = uid
	i.Igid = gid
	i.Is = size
	i.Iatime = now
	i.Ictime = now
	i.Imtime = now
	i.Itype = fileType // CAMBIO: usar byte
	i.Iperm = perm     // CAMBIO: usar [3]byte
	// Inicializar todos los bloques como no usados
	for j := 0; j < 16; j++ {
		i.Iblock[j] = -1
	}
}

// ===== GETTERS =====

// GetIblock retorna el array de bloques
func (i *Inodo) GetIblock() [16]int32 {
	return i.Iblock
}

// GetIuid retorna el ID del usuario
func (i *Inodo) GetIuid() int32 {
	return i.Iuid
}

// GetIgid retorna el ID del grupo
func (i *Inodo) GetIgid() int32 {
	return i.Igid
}

// GetIs retorna el tamaño
func (i *Inodo) GetIs() int32 {
	return i.Is
}

// GetItype retorna el tipo de inodo
func (i *Inodo) GetItype() byte { // CAMBIO: retornar byte
	return i.Itype
}

// GetIperm retorna los permisos
func (i *Inodo) GetIperm() [3]byte { // CAMBIO: retornar [3]byte
	return i.Iperm
}

// GetIatime retorna el tiempo de acceso
func (i *Inodo) GetIatime() int64 {
	return i.Iatime
}

// GetIctime retorna el tiempo de creación
func (i *Inodo) GetIctime() int64 {
	return i.Ictime
}

// GetImtime retorna el tiempo de modificación
func (i *Inodo) GetImtime() int64 {
	return i.Imtime
}

// ===== SETTERS =====

func (i *Inodo) SetIblock(index int, value int32) {
	if index >= 0 && index < 16 {
		i.Iblock[index] = value
	}
}

// ===== SETTERS DE BLOQUES =====

// SetBlock establece un bloque específico por índice
func (i *Inodo) SetBlock(index int, blockNum int32) {
	if index >= 0 && index < 16 {
		i.Iblock[index] = blockNum
	}
}

// SetDirectBlock establece un bloque directo (índices 0-11)
func (i *Inodo) SetDirectBlock(index int, blockNum int32) {
	if index >= 0 && index < 12 {
		i.Iblock[index] = blockNum
	}
}

// ReplaceAllBlocks reemplaza todo el array de bloques
func (i *Inodo) ReplaceAllBlocks(blocks [16]int32) {
	i.Iblock = blocks
}

// ClearAllBlocks inicializa todos los bloques como no usados
func (i *Inodo) ClearAllBlocks() {
	for j := 0; j < 16; j++ {
		i.Iblock[j] = -1
	}
}

// AddDirectBlock agrega un bloque directo en la primera posición libre
func (i *Inodo) AddDirectBlock(blockNum int32) bool {
	for j := 0; j < 12; j++ {
		if i.Iblock[j] == -1 {
			i.Iblock[j] = blockNum
			return true
		}
	}
	return false // No hay espacio libre en bloques directos
}

// SetUID establece el ID del usuario
func (i *Inodo) SetUID(uid int32) {
	i.Iuid = uid
}

// SetGID establece el ID del grupo
func (i *Inodo) SetGID(gid int32) {
	i.Igid = gid
}

// SetSize establece el tamaño
func (i *Inodo) SetSize(size int32) {
	i.Is = size
}

// SetType establece el tipo
func (i *Inodo) SetType(fileType byte) {
	i.Itype = fileType
}

// SetPermissions establece los permisos
func (i *Inodo) SetPermissions(perm [3]byte) {
	i.Iperm = perm
}

// SetPermissionsOctal establece los permisos por separado
func (i *Inodo) SetPermissionsOctal(user, group, other byte) {
	i.Iperm[0] = user
	i.Iperm[1] = group
	i.Iperm[2] = other
}

// ===== FUNCIONES DE TIEMPO =====

// UpdateAccessTime actualiza el tiempo de acceso
func (i *Inodo) UpdateAccessTime() {
	i.Iatime = GetCurrentTimestamp()
}

// UpdateModificationTime actualiza el tiempo de modificación
func (i *Inodo) UpdateModificationTime() {
	i.Imtime = GetCurrentTimestamp()
}

// ===== FUNCIONES AUXILIARES =====

// IsFile verifica si el inodo es un archivo
func (i *Inodo) IsFile() bool {
	return i.Itype == 1
}

// IsDirectory verifica si el inodo es una carpeta
func (i *Inodo) IsDirectory() bool {
	return i.Itype == 0
}

// SetAsFile configura el inodo como archivo
func (i *Inodo) SetAsFile() {
	i.Itype = 1
}

// SetAsDirectory configura el inodo como carpeta
func (i *Inodo) SetAsDirectory() {
	i.Itype = 0
}

// ===== FUNCIONES DE BLOQUES =====

// GetDirectBlocks retorna solo los bloques directos (primeros 12)
func (i *Inodo) GetDirectBlocks() []int32 {
	blocks := make([]int32, 0, 12)
	for j := 0; j < 12; j++ {
		if i.Iblock[j] != -1 {
			blocks = append(blocks, i.Iblock[j])
		}
	}
	return blocks
}

// GetIndirectBlock retorna el bloque simple indirecto
func (i *Inodo) GetIndirectBlock() int32 {
	return i.Iblock[12]
}

// GetDoubleIndirectBlock retorna el bloque doble indirecto
func (i *Inodo) GetDoubleIndirectBlock() int32 {
	return i.Iblock[13]
}

// GetTripleIndirectBlock retorna el bloque triple indirecto
func (i *Inodo) GetTripleIndirectBlock() int32 {
	return i.Iblock[14]
}

// SetIndirectBlock establece el bloque simple indirecto
func (i *Inodo) SetIndirectBlock(blockNum int32) {
	i.Iblock[12] = blockNum
}

// SetDoubleIndirectBlock establece el bloque doble indirecto
func (i *Inodo) SetDoubleIndirectBlock(blockNum int32) {
	i.Iblock[13] = blockNum
}

// SetTripleIndirectBlock establece el bloque triple indirecto
func (i *Inodo) SetTripleIndirectBlock(blockNum int32) {
	i.Iblock[14] = blockNum
}

// CountUsedBlocks cuenta cuántos bloques directos están siendo usados
func (i *Inodo) CountUsedBlocks() int {
	count := 0
	for j := 0; j < 16; j++ {
		if i.Iblock[j] != -1 {
			count++
		}
	}
	return count
}

// HasIndirectBlocks verifica si usa bloques indirectos
func (i *Inodo) HasIndirectBlocks() bool {
	return i.Iblock[12] != -1 || i.Iblock[13] != -1 || i.Iblock[14] != -1
}

// ===== FUNCIONES DE PERMISOS =====

// GetPermissionsString retorna los permisos como string legible
func (i *Inodo) GetPermissionsString() string {
	perms := ""

	// Tipo de archivo
	if i.IsDirectory() {
		perms += "d"
	} else {
		perms += "-"
	}

	// Permisos del usuario (owner)
	user := i.Iperm[0]
	perms += formatPermissionBits(user)

	// Permisos del grupo
	group := i.Iperm[1]
	perms += formatPermissionBits(group)

	// Permisos de otros
	other := i.Iperm[2]
	perms += formatPermissionBits(other)

	return perms
}

// formatPermissionBits convierte un byte de permisos a string rwx
func formatPermissionBits(perm byte) string {
	result := ""

	if perm&4 != 0 { // Bit de lectura
		result += "r"
	} else {
		result += "-"
	}

	if perm&2 != 0 { // Bit de escritura
		result += "w"
	} else {
		result += "-"
	}

	if perm&1 != 0 { // Bit de ejecución
		result += "x"
	} else {
		result += "-"
	}

	return result
}

// String retorna una representación en string del inodo para debug
func (i *Inodo) String() string {
	return fmt.Sprintf("Inodo{UID:%d, GID:%d, Size:%d, Type:%d, Perms:%v, Blocks:%v}",
		i.Iuid, i.Igid, i.Is, i.Itype, i.Iperm, i.Iblock)
}

// PrintInfo imprime información detallada del inodo
func (i *Inodo) PrintInfo() {
	fmt.Printf("=== INFORMACIÓN DEL INODO ===\n")
	fmt.Printf("UID: %d\n", i.Iuid)
	fmt.Printf("GID: %d\n", i.Igid)
	fmt.Printf("Tamaño: %d bytes\n", i.Is)
	fmt.Printf("Tipo: %s\n", map[byte]string{0: "Carpeta", 1: "Archivo"}[i.Itype])
	fmt.Printf("Permisos: %s (%d%d%d)\n", i.GetPermissionsString(), i.Iperm[0], i.Iperm[1], i.Iperm[2])
	fmt.Printf("Fecha creación: %d\n", i.Ictime)
	fmt.Printf("Fecha modificación: %d\n", i.Imtime)
	fmt.Printf("Fecha acceso: %d\n", i.Iatime)
	fmt.Printf("Bloques usados: %d/16\n", i.CountUsedBlocks())
	fmt.Printf("Bloques directos: %v\n", i.GetDirectBlocks())
	if i.HasIndirectBlocks() {
		fmt.Printf("Bloque indirecto simple: %d\n", i.GetIndirectBlock())
		fmt.Printf("Bloque indirecto doble: %d\n", i.GetDoubleIndirectBlock())
		fmt.Printf("Bloque indirecto triple: %d\n", i.GetTripleIndirectBlock())
	}
	fmt.Printf("==============================\n")
}
