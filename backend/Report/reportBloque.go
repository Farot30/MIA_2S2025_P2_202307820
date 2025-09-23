package reportP2

import (
	bloques "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt2P2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	estructuraArchivoCarpeta "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"
)

type Edge struct {
	From int32
	To   int32
}

type InodeBlockMapping struct {
	InodeNum  int32
	BlockNums []int32
	IsFolder  bool
}

func GenerarBloqueReporte(diskPath string, partitionStart int32, outputPath string) error {
	file, err := os.OpenFile(diskPath, os.O_RDONLY, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", diskPath, err)
	}
	defer file.Close()

	ext2fs, err := EstructuraExt2P2.ReadEXT2FileSystem(diskPath, partitionStart)
	if err != nil {
		return fmt.Errorf("error leyendo sistema de archivos EXT2: %s", err)
	}

	if ext2fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return fmt.Errorf("el sistema de archivos no es EXT2 válido (magic number incorrecto)")
	}

	// Crear mapeo de inodos a bloques para mejor detección
	inodeMapping := buildInodeBlockMapping(ext2fs)

	var dot strings.Builder
	var edges []Edge

	dot.WriteString("digraph BLOCK_REPORT {\n")
	dot.WriteString("  node [shape=plaintext, style=filled];\n")
	dot.WriteString("  rankdir=TB;\n")
	dot.WriteString("  label=\"Reporte de Bloques Utilizados\";\n")
	dot.WriteString("  labelloc=\"t\";\n")
	dot.WriteString("  bgcolor=\"white\";\n\n")

	// Agregar nodo del bloque raíz primero
	generateRootBlockNode(&dot)

	bitmapBlockSize := (ext2fs.SuperBlock.S_blocks_count + 7) / 8
	bitmapBlockData := make([]byte, bitmapBlockSize)

	_, err = file.Seek(int64(ext2fs.SuperBlock.S_bm_block_start), 0)
	if err != nil {
		return fmt.Errorf("error posicionándose en el bitmap de bloques: %s", err)
	}

	_, err = file.Read(bitmapBlockData)
	if err != nil {
		return fmt.Errorf("error leyendo el bitmap de bloques: %s", err)
	}

	blockTypes := mapBlockTypesWithInodeInfo(ext2fs, inodeMapping)

	// Procesar solo bloques utilizados según el bitmap
	for blockNum := int32(0); blockNum < ext2fs.SuperBlock.S_blocks_count; blockNum++ {
		bytePos := blockNum / 8
		bitPos := blockNum % 8

		if bytePos < int32(len(bitmapBlockData)) {
			mask := byte(1 << (7 - bitPos))
			if (bitmapBlockData[bytePos] & mask) != 0 {
				blockOffset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
				block := make([]byte, ext2fs.SuperBlock.S_block_s)
				_, err := file.Seek(blockOffset, 0)
				if err != nil {
					return fmt.Errorf("error posicionándose en el bloque %d: %s", blockNum, err)
				}
				_, err = file.Read(block)
				if err != nil {
					return fmt.Errorf("error leyendo el bloque %d: %s", blockNum, err)
				}

				generateImprovedBlockNode(&dot, block, blockNum, blockTypes[blockNum], &edges, ext2fs, inodeMapping)
			}
		}
	}

	rootInodeNum := int32(0) // Buscar dinámicamente el inodo raíz
	rootInode, err := ext2fs.ReadInode(rootInodeNum)
	if err == nil && rootInode.Iblock[0] != -1 && rootInode.Iblock[0] >= 0 {
		edges = append(edges, Edge{From: -1, To: rootInode.Iblock[0]}) // Root virtual -> primer bloque del directorio raíz
	}

	// Agregar edges al DOT con mejor formato
	dot.WriteString("\n  // Conexiones entre bloques\n")
	for _, edge := range edges {
		if edge.From == -1 {
			dot.WriteString(fmt.Sprintf("  Root -> Bloque%d [color=darkgreen, penwidth=2];\n", edge.To))
		} else {
			dot.WriteString(fmt.Sprintf("  Bloque%d -> Bloque%d [color=blue];\n", edge.From, edge.To))
		}
	}

	dot.WriteString("}\n")

	dotFile := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".dot"
	if err := os.WriteFile(dotFile, []byte(dot.String()), 0644); err != nil {
		return fmt.Errorf("error escribiendo archivo DOT: %s", err)
	}

	ext := filepath.Ext(outputPath)
	if ext == "" {
		outputPath += ".png"
		ext = ".png"
	}
	format := strings.TrimPrefix(ext, ".")
	cmd := exec.Command("dot", "-T"+format, dotFile, "-o", outputPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error generando imagen con Graphviz: %s", err)
	}

	return nil
}

// generateRootBlockNode genera el nodo virtual del sistema raíz con mejor estilo
func generateRootBlockNode(dot *strings.Builder) {
	dot.WriteString(`  Root [label=<
<TABLE BORDER="2" CELLBORDER="1" CELLSPACING="0" CELLPADDING="4">
  <TR><TD><B>Directorio Raíz (/)</B></TD></TR>
  <TR><TD><FONT POINT-SIZE="10">Punto de inicio del sistema</FONT></TD></TR>
</TABLE>>, fillcolor="lightgreen", style="filled,bold"];
`)
}

func findRootDirectoryInode(ext2fs *EstructuraExt2P2.EXT2FileSystem) int32 {
	// En EXT2, el inodo 2 es SIEMPRE el directorio raíz por estándar
	// Primero verificar que el inodo 2 existe y es un directorio
	if inode, err := ext2fs.ReadInode(2); err == nil {
		if inode.Itype == 0 && inode.Iblock[0] >= 0 {
			return 2
		}
	}

	// Si el inodo 2 no funciona, buscar el primer directorio que parezca raíz
	for inodeNum := int32(0); inodeNum < ext2fs.SuperBlock.S_inodes_count; inodeNum++ {
		if inode, err := ext2fs.ReadInode(inodeNum); err == nil && inode.Itype == 0 {
			if isRootDirectory(ext2fs, inodeNum) {
				return inodeNum
			}
		}
	}

	return 2 // Fallback al estándar EXT2
}

func isRootDirectory(ext2fs *EstructuraExt2.EXT2FileSystem, inodeNum int32) bool {
	inode, err := ext2fs.ReadInode(inodeNum)
	if err != nil || inode.Iblock[0] == -1 || inode.Iblock[0] < 0 {
		return false
	}

	// Leer el primer bloque del directorio
	file, err := os.OpenFile(ext2fs.PartitionPath, os.O_RDONLY, 0666)
	if err != nil {
		return false
	}
	defer file.Close()

	blockOffset := int64(ext2fs.SuperBlock.S_block_start + (inode.Iblock[0] * ext2fs.SuperBlock.S_block_s))
	block := make([]byte, ext2fs.SuperBlock.S_block_s)

	_, err = file.Seek(blockOffset, 0)
	if err != nil {
		return false
	}

	_, err = file.Read(block)
	if err != nil {
		return false
	}

	// Parsear como bloque de directorio
	bloqueCarpeta := &bloques.BloqueCarpeta{}
	if len(block) >= int(bloqueCarpeta.GetSize()) {
		copy((*[64]byte)(unsafe.Pointer(bloqueCarpeta))[:], block[:bloqueCarpeta.GetSize()])
	} else {
		copy((*[64]byte)(unsafe.Pointer(bloqueCarpeta))[:], block)
	}

	// Un directorio raíz debe tener "." apuntando a sí mismo
	// y típicamente contener entradas como "home", "users.txt", etc.
	hasSelfReference := false
	hasTypicalRootEntries := false

	for _, entry := range bloqueCarpeta.B_content {
		name := strings.TrimRight(string(entry.B_name[:]), "\x00")
		if name == "." && entry.B_inodo == inodeNum {
			hasSelfReference = true
		}
		// Buscar entradas típicas del directorio raíz
		if name == "home" || name == "users.txt" || name == "user" {
			hasTypicalRootEntries = true
		}
	}

	return hasSelfReference && hasTypicalRootEntries
}

func isRealFolderBlock(block []byte, ext2fs *EstructuraExt2P2.EXT2FileSystem) bool {
	if len(block) < 64 {
		return false
	}

	// Intentar parsear como bloque de carpeta
	bloqueCarpeta := &bloques.BloqueCarpeta{}
	if len(block) >= int(bloqueCarpeta.GetSize()) {
		copy((*[64]byte)(unsafe.Pointer(bloqueCarpeta))[:], block[:bloqueCarpeta.GetSize()])
	} else {
		copy((*[64]byte)(unsafe.Pointer(bloqueCarpeta))[:], block)
	}

	validEntries := 0
	totalEntries := 0

	for _, entry := range bloqueCarpeta.B_content {
		name := strings.TrimRight(string(entry.B_name[:]), "\x00")

		if name != "" {
			totalEntries++

			// Entrada válida: nombre no vacío y inodo en rango válido
			if (entry.B_inodo >= 0 && entry.B_inodo < ext2fs.SuperBlock.S_inodes_count) ||
				name == "." || name == ".." {
				validEntries++
			}

			// Verificar que los nombres son alfanuméricos válidos (no basura)
			validName := true
			for _, char := range name {
				if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
					(char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-') {
					validName = false
					break
				}
			}
			if !validName {
				return false
			}
		}
	}

	// Es un directorio válido si tiene al menos una entrada válida
	// y la mayoría de entradas no vacías son válidas
	return totalEntries > 0 && validEntries >= totalEntries
}

// buildInodeBlockMapping construye un mapeo de inodos a sus bloques (mejorado)
func buildInodeBlockMapping(ext2fs *EstructuraExt2P2.EXT2FileSystem) map[int32]*InodeBlockMapping {
	mapping := make(map[int32]*InodeBlockMapping)

	// Leer todos los inodos activos
	for inodeNum := int32(0); inodeNum < ext2fs.SuperBlock.S_inodes_count; inodeNum++ {
		inode, err := ext2fs.ReadInode(inodeNum)
		if err != nil {
			continue
		}

		// Solo procesar inodos que están en uso (tienen bloques asignados)
		var blocks []int32
		for i := 0; i < 15; i++ { // 12 directos + 3 indirectos
			if inode.Iblock[i] != -1 && inode.Iblock[i] >= 0 && inode.Iblock[i] < ext2fs.SuperBlock.S_blocks_count {
				blocks = append(blocks, inode.Iblock[i])
			}
		}

		if len(blocks) > 0 {
			mapping[inodeNum] = &InodeBlockMapping{
				InodeNum:  inodeNum,
				BlockNums: blocks,
				IsFolder:  inode.Itype == 0, // 0 = carpeta, 1 = archivo
			}
		}
	}

	return mapping
}

func generateImprovedBlockNode(dot *strings.Builder, block []byte, blockNum int32, blockType BlockType, edges *[]Edge, ext2fs *EstructuraExt2P2.EXT2FileSystem, inodeMapping map[int32]*InodeBlockMapping) {

	// CORRECCIÓN ESPECÍFICA PARA EL BLOQUE 0
	if blockNum == 0 {
		// Intentar primero como bloque de carpeta
		if isRealFolderBlock(block, ext2fs) {
			generateFolderBlockNode(dot, block, blockNum, edges, ext2fs)
			return
		}
	}

	// Mantener la lógica original para todos los demás bloques
	switch blockType {
	case BlockCarpeta:
		generateFolderBlockNode(dot, block, blockNum, edges, ext2fs)
	case BlockArchivo:
		generateFileBlockNodes(dot, block, blockNum)
	case BlockApuntador:
		generatePointerBlockNode(dot, block, blockNum, edges, ext2fs)
	default:
		generateUnknownBlockNode(dot, block, blockNum)
	}
}

func generateFolderBlockNode(dot *strings.Builder, block []byte, blockNum int32, edges *[]Edge, ext2fs *EstructuraExt2P2.EXT2FileSystem) {
	bloqueCarpeta := &bloques.BloqueCarpeta{}

	if int32(len(block)) >= bloqueCarpeta.GetSize() {
		data := block[:bloqueCarpeta.GetSize()]
		copy((*[64]byte)(unsafe.Pointer(bloqueCarpeta))[:], data)
	} else {
		copy((*[64]byte)(unsafe.Pointer(bloqueCarpeta))[:], block)
	}

	label := fmt.Sprintf(
		`<
<TABLE BORDER="2" CELLBORDER="1" CELLSPACING="0" CELLPADDING="4">
  <TR><TD COLSPAN="2"><B>Bloque Carpeta %d</B></TD></TR>
  <TR><TD><B>b_name</B></TD><TD><B>b_inodo</B></TD></TR>`, blockNum)

	for _, entry := range bloqueCarpeta.B_content {
		name := strings.TrimRight(string(entry.B_name[:]), "\x00")
		if name != "" && (entry.B_inodo >= 0 || name == "." || name == "..") {
			label += fmt.Sprintf("<TR><TD>%s</TD><TD>%d</TD></TR>", escapeHTML(name), entry.B_inodo)

			if entry.B_inodo >= 0 && entry.B_inodo < ext2fs.SuperBlock.S_inodes_count && name != "." && name != ".." {
				// En lugar de conectar todos los bloques del inodo,
				// solo conectar al primer bloque del inodo referenciado
				targetInode, err := ext2fs.ReadInode(entry.B_inodo)
				if err == nil && targetInode.Iblock[0] != -1 && targetInode.Iblock[0] >= 0 {
					*edges = append(*edges, Edge{From: blockNum, To: targetInode.Iblock[0]})

					// Si el archivo tiene múltiples bloques, conectarlos secuencialmente
					connectSequentialBlocks(edges, entry.B_inodo, ext2fs)
				}
			}
		}
	}
	label += "</TABLE>>"
	dot.WriteString(fmt.Sprintf("  Bloque%d [label=%s, fillcolor=\"lightblue\", style=\"filled,bold\"];\n", blockNum, label))
}

func connectSequentialBlocks(edges *[]Edge, inodeNum int32, ext2fs *EstructuraExt2P2.EXT2FileSystem) {
	inode, err := ext2fs.ReadInode(inodeNum)
	if err != nil {
		return
	}

	var previousBlock int32 = -1

	// Conectar bloques directos secuencialmente (0-11)
	for i := 0; i < 12; i++ {
		if inode.Iblock[i] != -1 && inode.Iblock[i] >= 0 && inode.Iblock[i] < ext2fs.SuperBlock.S_blocks_count {
			if previousBlock != -1 {
				*edges = append(*edges, Edge{From: previousBlock, To: inode.Iblock[i]})
			}
			previousBlock = inode.Iblock[i]
		}
	}

	// Manejar bloques indirectos
	if inode.Iblock[12] != -1 && inode.Iblock[12] >= 0 && inode.Iblock[12] < ext2fs.SuperBlock.S_blocks_count {
		if previousBlock != -1 {
			*edges = append(*edges, Edge{From: previousBlock, To: inode.Iblock[12]})
		}
		connectIndirectBlocks(edges, inode.Iblock[12], ext2fs, 1)
	}

	if inode.Iblock[13] != -1 && inode.Iblock[13] >= 0 && inode.Iblock[13] < ext2fs.SuperBlock.S_blocks_count {
		if previousBlock != -1 {
			*edges = append(*edges, Edge{From: previousBlock, To: inode.Iblock[13]})
		}
		connectIndirectBlocks(edges, inode.Iblock[13], ext2fs, 2)
	}

	if inode.Iblock[14] != -1 && inode.Iblock[14] >= 0 && inode.Iblock[14] < ext2fs.SuperBlock.S_blocks_count {
		if previousBlock != -1 {
			*edges = append(*edges, Edge{From: previousBlock, To: inode.Iblock[14]})
		}
		connectIndirectBlocks(edges, inode.Iblock[14], ext2fs, 3)
	}
}

func generateFileBlockNodes(dot *strings.Builder, block []byte, blockNum int32) {
	bloqueArchivo := &bloques.BloqueArchivo{}

	if int32(len(block)) >= bloqueArchivo.GetSize() {
		copy((*[64]byte)(unsafe.Pointer(bloqueArchivo))[:], block[:bloqueArchivo.GetSize()])
	} else {
		copy((*[64]byte)(unsafe.Pointer(bloqueArchivo))[:], block)
	}

	content := strings.TrimRight(bloqueArchivo.GetContent(), "\x00")
	content = escapeHTML(content)
	content = strings.ReplaceAll(content, "\n", "<BR/>")

	// Limitar contenido mostrado para evitar nodos muy grandes
	if len(content) > 80 {
		content = content[:80] + "..."
	}

	label := fmt.Sprintf(
		`<
<TABLE BORDER="2" CELLBORDER="1" CELLSPACING="0" CELLPADDING="4">
  <TR><TD><B>Bloque Archivo %d</B></TD></TR>
  <TR><TD><FONT POINT-SIZE="10">%s</FONT></TD></TR>
</TABLE>>`, blockNum, content)

	dot.WriteString(fmt.Sprintf("  Bloque%d [label=%s, fillcolor=\"lightyellow\", style=\"filled\"];\n", blockNum, label))
}

func parseBlockPointersManually(bloqueApuntador *bloques.BloqueApuntador, block []byte, ext2fs *EstructuraExt2P2.EXT2FileSystem) bool {
	if len(block) < 4 {
		return false
	}

	pointersCount := len(block) / 4
	if pointersCount > len(bloqueApuntador.B_pointers) {
		pointersCount = len(bloqueApuntador.B_pointers)
	}

	validParsed := 0
	for i := 0; i < pointersCount; i++ {
		offset := i * 4
		if offset+4 <= len(block) {
			// Leer como little endian
			pointer := int32(block[offset]) |
				int32(block[offset+1])<<8 |
				int32(block[offset+2])<<16 |
				int32(block[offset+3])<<24

			bloqueApuntador.B_pointers[i] = pointer

			// Contar apuntadores válidos o nulos
			if pointer == -1 || (pointer >= 0 && pointer < ext2fs.SuperBlock.S_blocks_count) {
				validParsed++
			}
		}
	}

	// Debe tener al menos algunos apuntadores válidos o nulos
	return validParsed > 0
}

func isContextualPointerBlock(block []byte, blockNum int32, ext2fs *EstructuraExt2P2.EXT2FileSystem) bool {
	// Verificar si este bloque está referenciado como un bloque indirecto por algún inodo
	for inodeNum := int32(0); inodeNum < ext2fs.SuperBlock.S_inodes_count; inodeNum++ {
		inode, err := ext2fs.ReadInode(inodeNum)
		if err != nil {
			continue
		}

		// Verificar si este bloque está en las posiciones de bloques indirectos (12, 13, 14)
		for i := 12; i < 15; i++ {
			if inode.Iblock[i] == blockNum {
				return true // Este bloque está referenciado como indirecto
			}
		}
	}
	return false
}

func generatePointerBlockNode(dot *strings.Builder, block []byte, blockNum int32, edges *[]Edge, ext2fs *EstructuraExt2P2.EXT2FileSystem) {
	// Validar que realmente es un bloque de apuntadores con criterios más flexibles
	if !isValidPointerBlock(block, ext2fs) {
		// Si no es detectado como bloque de apuntadores, intentar una verificación alternativa
		// basada en el contexto del sistema de archivos
		if !isContextualPointerBlock(block, blockNum, ext2fs) {
			generateFileBlockNodes(dot, block, blockNum)
			return
		}
	}

	bloqueApuntador := &bloques.BloqueApuntador{}

	// Asegurar que tenemos suficientes bytes
	blockSize := int(ext2fs.SuperBlock.S_block_s)
	if len(block) < blockSize {
		// Extender el bloque con ceros si es necesario
		extendedBlock := make([]byte, blockSize)
		copy(extendedBlock, block)
		block = extendedBlock
	}

	if err := bloqueApuntador.FromBytes(block[:bloqueApuntador.GetSize()]); err != nil {
		// Si falla el parseo, intentar parseo manual
		if !parseBlockPointersManually(bloqueApuntador, block, ext2fs) {
			generateUnknownBlockNode(dot, block, blockNum)
			return
		}
	}

	label := fmt.Sprintf(
		`<
<TABLE BORDER="2" CELLBORDER="1" CELLSPACING="0" CELLPADDING="4">
  <TR><TD COLSPAN="4"><B>Bloque Apuntadores %d</B></TD></TR>`, blockNum)

	validConnections := 0
	// Organizar apuntadores en filas de 4
	for i := 0; i < len(bloqueApuntador.B_pointers); i += 4 {
		label += "<TR>"
		for j := 0; j < 4 && i+j < len(bloqueApuntador.B_pointers); j++ {
			pointer := bloqueApuntador.B_pointers[i+j]
			val := "-1"
			if pointer >= 0 && pointer < ext2fs.SuperBlock.S_blocks_count {
				val = fmt.Sprintf("%d", pointer)
				*edges = append(*edges, Edge{From: blockNum, To: pointer})
				validConnections++
			}
			label += fmt.Sprintf("<TD>%s</TD>", val)
		}
		// Completar la fila si es necesario
		for k := (i + 4) - len(bloqueApuntador.B_pointers); k > 0 && k < 4; k++ {
			label += "<TD>-1</TD>"
		}
		label += "</TR>"

		// Solo mostrar las primeras filas para mantener el nodo legible
		if i >= 12 {
			break
		}
	}

	label += "</TABLE>>"

	// Si no hay conexiones válidas, podría no ser realmente un bloque de apuntadores
	if validConnections == 0 {
		generateUnknownBlockNode(dot, block, blockNum)
		return
	}

	dot.WriteString(fmt.Sprintf("  Bloque%d [label=%s, fillcolor=\"lightcoral\", style=\"filled\"];\n", blockNum, label))
}

func generateUnknownBlockNode(dot *strings.Builder, block []byte, blockNum int32) {
	// Mostrar contenido como texto si es posible, con mejor formato
	content := strings.TrimRight(string(block), "\x00")
	content = escapeHTML(content)
	content = strings.ReplaceAll(content, "\n", "<BR/>")

	if len(content) > 60 {
		content = content[:60] + "..."
	}

	if content == "" {
		content = "[Bloque vacío o binario]"
	}

	label := fmt.Sprintf(
		`<
<TABLE BORDER="2" CELLBORDER="1" CELLSPACING="0" CELLPADDING="4">
  <TR><TD><B>Bloque %d</B></TD></TR>
  <TR><TD><FONT POINT-SIZE="10">%s</FONT></TD></TR>
</TABLE>>`, blockNum, content)

	dot.WriteString(fmt.Sprintf("  Bloque%d [label=%s, fillcolor=\"lightgray\", style=\"filled\"];\n", blockNum, label))
}

func isValidPointerBlock(block []byte, ext2fs *EstructuraExt2P2.EXT2FileSystem) bool {
	if len(block) < 4 {
		return false
	}

	validPointers := 0
	nullPointers := 0
	totalChecked := 0
	consecutiveNulls := 0
	maxConsecutiveNulls := 0

	// Revisar todos los enteros posibles en el bloque (no solo los primeros 16)
	maxPointers := len(block) / 4
	if maxPointers > 16 {
		maxPointers = 16 // Limitar para eficiencia, pero revisar más que antes
	}

	for i := 0; i < maxPointers; i++ {
		offset := i * 4
		if offset+4 > len(block) {
			break
		}

		// Leer como little endian
		value := int32(block[offset]) |
			int32(block[offset+1])<<8 |
			int32(block[offset+2])<<16 |
			int32(block[offset+3])<<24

		totalChecked++

		if value == -1 {
			nullPointers++
			consecutiveNulls++
			if consecutiveNulls > maxConsecutiveNulls {
				maxConsecutiveNulls = consecutiveNulls
			}
		} else {
			consecutiveNulls = 0
			if value >= 0 && value < ext2fs.SuperBlock.S_blocks_count {
				validPointers++
			}
		}
	}

	// No más del 90% de valores nulos consecutivos (permite más flexibilidad)
	// La mayoría de valores deben ser apuntadores válidos o -1
	// No debe ser principalmente texto ASCII

	hasValidPointers := validPointers >= 1
	notTooManyConsecutiveNulls := maxConsecutiveNulls < (totalChecked * 9 / 10)
	mostlyValidOrNull := (validPointers + nullPointers) >= (totalChecked * 2 / 3) // Más flexible: 66% en lugar de 75%
	notTextBlock := !isTextBlock(block)

	return hasValidPointers && notTooManyConsecutiveNulls && mostlyValidOrNull && notTextBlock
}

// isTextBlock detecta si un bloque contiene principalmente texto
func isTextBlock(block []byte) bool {
	if len(block) == 0 {
		return false
	}

	textBytes := 0
	printableBytes := 0

	// Revisar solo los primeros 64 bytes para eficiencia
	checkLength := len(block)
	if checkLength > 64 {
		checkLength = 64
	}

	for i := 0; i < checkLength; i++ {
		b := block[i]
		// Caracteres imprimibles ASCII
		if b >= 32 && b <= 126 {
			printableBytes++
			textBytes++
		} else if b == 10 || b == 13 || b == 9 { // \n, \r, \t
			textBytes++
		} else if b == 0 {
			// Permitir algunos nulos, pero no demasiados
			textBytes++
		}
	}

	// Es texto si más del 80% son caracteres válidos de texto
	// Y si hay una cantidad significativa de caracteres imprimibles
	isText := textBytes > (checkLength*4/5) && printableBytes > (checkLength/4)

	return isText
}

func connectIndirectBlocks(edges *[]Edge, pointerBlockNum int32, ext2fs *EstructuraExt2P2.EXT2FileSystem, level int) {
	if level <= 0 || pointerBlockNum < 0 || pointerBlockNum >= ext2fs.SuperBlock.S_blocks_count {
		return
	}

	// Leer el bloque de apuntadores
	file, err := os.OpenFile(ext2fs.PartitionPath, os.O_RDONLY, 0666)
	if err != nil {
		return
	}
	defer file.Close()

	blockOffset := int64(ext2fs.SuperBlock.S_block_start + (pointerBlockNum * ext2fs.SuperBlock.S_block_s))
	block := make([]byte, ext2fs.SuperBlock.S_block_s)
	_, err = file.Seek(blockOffset, 0)
	if err != nil {
		return
	}
	_, err = file.Read(block)
	if err != nil {
		return
	}

	// Parsear apuntadores en el bloque
	pointersPerBlock := int(ext2fs.SuperBlock.S_block_s) / 4
	for i := 0; i < pointersPerBlock; i++ {
		offset := i * 4
		if offset+4 <= len(block) {
			// Leer como little endian
			pointer := int32(block[offset]) |
				int32(block[offset+1])<<8 |
				int32(block[offset+2])<<16 |
				int32(block[offset+3])<<24

			if pointer >= 0 && pointer < ext2fs.SuperBlock.S_blocks_count {
				*edges = append(*edges, Edge{From: pointerBlockNum, To: pointer})

				// Si es nivel > 1, recursivamente conectar los sub-bloques
				if level > 1 {
					connectIndirectBlocks(edges, pointer, ext2fs, level-1)
				}
			}
		}
	}
}

func mapBlockTypesWithInodeInfo(ext2fs *EstructuraExt2P2.EXT2FileSystem, inodeMapping map[int32]*InodeBlockMapping) map[int32]BlockType {
	blockTypes := make(map[int32]BlockType)

	// Primero, mapear bloques conocidos desde los inodos
	for _, mapping := range inodeMapping {
		inode, err := ext2fs.ReadInode(mapping.InodeNum)
		if err != nil {
			continue
		}

		// Para carpetas, todos los bloques directos son bloques de carpeta
		if mapping.IsFolder {
			for i := 0; i < 12 && i < len(mapping.BlockNums); i++ {
				if mapping.BlockNums[i] >= 0 {
					blockTypes[mapping.BlockNums[i]] = BlockCarpeta
				}
			}
		} else {
			// Para archivos, los bloques directos (0-11) son datos
			for i := 0; i < 12 && i < len(mapping.BlockNums); i++ {
				if mapping.BlockNums[i] >= 0 {
					blockTypes[mapping.BlockNums[i]] = BlockArchivo
				}
			}
		}

		// Mapear bloques indirectos específicamente
		// Bloque indirecto simple (índice 12)
		if inode.Iblock[12] != -1 && inode.Iblock[12] >= 0 {
			blockTypes[inode.Iblock[12]] = BlockApuntador
		}

		// Bloque indirecto doble (índice 13)
		if inode.Iblock[13] != -1 && inode.Iblock[13] >= 0 {
			blockTypes[inode.Iblock[13]] = BlockApuntador
		}

		// Bloque indirecto triple (índice 14)
		if inode.Iblock[14] != -1 && inode.Iblock[14] >= 0 {
			blockTypes[inode.Iblock[14]] = BlockApuntador
		}
	}

	// CORRECCIÓN ESPECÍFICA: Verificar manualmente el bloque 0
	// Si no fue clasificado correctamente por el mapeo de inodos
	if blockTypes[0] != BlockCarpeta {
		// Leer directamente el bloque 0 y verificar si es directorio
		file, err := os.OpenFile(ext2fs.PartitionPath, os.O_RDONLY, 0666)
		if err != nil {
			fmt.Printf("Error abriendo archivo: %v\n", err)
			return blockTypes // Devuelve el mapa actual, aunque incompleto
		}
		defer file.Close()

		blockOffset := int64(ext2fs.SuperBlock.S_block_start)
		block := make([]byte, ext2fs.SuperBlock.S_block_s)

		// Posicionarse en el bloque 0
		_, err = file.Seek(blockOffset, 0)
		if err != nil {
			fmt.Printf("Error posicionándose en el bloque 0: %v\n", err)
			return blockTypes // Devuelve el mapa actual, aunque incompleto
		}

		// Leer el bloque 0
		_, err = file.Read(block)
		if err != nil {
			fmt.Printf("Error leyendo el bloque 0: %v\n", err)
			return blockTypes // Devuelve el mapa actual, aunque incompleto
		}

		// Verificar si el bloque es un directorio
		if isRealFolderBlock(block, ext2fs) {
			blockTypes[0] = BlockCarpeta
		}
	}

	return blockTypes
}
