package report

import (
	bloques "MIA_2S2025_P1_202307820/Bloques"
	EstructuraExt2 "MIA_2S2025_P1_202307820/EstructuraExt2"
	estructuraArchivoCarpeta "MIA_2S2025_P1_202307820/estructuraArchivoCarpeta"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unsafe"
)

const (
	fileCap = 64
)

// Estructuras para organizar la jerarquía
type NodeInfo struct {
	ID    int32
	Level int
	Type  string // "inode", "folder_block", "file_block", "pointer_block"
}

var nodeHierarchy map[int32]*NodeInfo
var maxDepth int
var levelNodes map[int][]int32

// GenerateTreeReport genera el reporte "tree" del sistema EXT2 con mejor organización jerárquica
func GenerateTreeReport(diskPath string, partitionStart int32, outputPath string) error {
	f, err := os.OpenFile(diskPath, os.O_RDONLY, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo disco: %v", err)
	}
	defer f.Close()

	ext2fs, err := EstructuraExt2.ReadEXT2FileSystem(diskPath, partitionStart)
	if err != nil {
		return fmt.Errorf("error leyendo EXT2: %v", err)
	}
	if ext2fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return fmt.Errorf("no es EXT2 válido")
	}

	// Inicializar estructuras
	nodeHierarchy = make(map[int32]*NodeInfo)
	levelNodes = make(map[int][]int32)
	maxDepth = 0

	var dot strings.Builder
	dot.WriteString("digraph TREE_REPORT {\n")
	// Configuración optimizada para árbol jerárquico
	dot.WriteString("  graph [bgcolor=\"white\", rankdir=TB, nodesep=0.8, ranksep=1.5, splines=ortho];\n")
	dot.WriteString("  node [shape=none, fontname=\"Arial\", fontsize=10];\n")
	dot.WriteString("  edge [fontname=\"Arial\", fontsize=8, arrowsize=0.8];\n")
	dot.WriteString("  labelloc=\"t\"; label=\"Reporte Tree - EXT2\"; labeljust=\"c\";\n\n")

	visitedInode := make(map[int32]bool)
	visitedBlock := make(map[int32]bool)

	// Primera pasada: construir jerarquía desde raíz
	if err := buildHierarchy(f, ext2fs, 0, 0, visitedInode, visitedBlock); err != nil {
		return fmt.Errorf("error construyendo jerarquía: %v", err)
	}

	// Segunda pasada: generar nodos y conexiones
	visitedInode = make(map[int32]bool)
	visitedBlock = make(map[int32]bool)

	if err := generateNodesAndEdges(f, ext2fs, 0, &dot, visitedInode, visitedBlock, -1); err != nil {
		return fmt.Errorf("error generando nodos: %v", err)
	}

	// Generar constraints de nivel
	generateImprovedLevelConstraints(&dot)

	dot.WriteString("}\n")

	// Escribir archivo y generar imagen
	dotFile := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".dot"
	if err := os.WriteFile(dotFile, []byte(dot.String()), 0644); err != nil {
		return fmt.Errorf("error escribiendo dot: %v", err)
	}

	ext := filepath.Ext(outputPath)
	if ext == "" {
		outputPath += ".png"
		ext = ".png"
	}
	format := strings.TrimPrefix(ext, ".")

	// Usar dot con configuración específica para árboles
	cmd := exec.Command("dot", "-T"+format, "-Gdpi=300", dotFile, "-o", outputPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error ejecutando dot: %v", err)
	}
	return nil
}

// buildHierarchy construye la estructura jerárquica sin generar el DOT
func buildHierarchy(f *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, inodeNum int32, depth int, visitedInode map[int32]bool, visitedBlock map[int32]bool) error {
	if visitedInode[inodeNum] || depth > 10 { // Prevenir recursión infinita
		return nil
	}
	visitedInode[inodeNum] = true

	// Registrar inodo en jerarquía
	nodeHierarchy[inodeNum] = &NodeInfo{ID: inodeNum, Level: depth, Type: "inode"}
	levelNodes[depth] = append(levelNodes[depth], inodeNum)

	if depth > maxDepth {
		maxDepth = depth
	}

	inode, err := ext2fs.ReadInode(inodeNum)
	if err != nil {
		return fmt.Errorf("leer inodo %d: %v", inodeNum, err)
	}

	if inode.IsDirectory() {
		// Procesar bloques de carpeta (código existente)
		for i := 0; i < 12; i++ {
			b := inode.Iblock[i]
			if b >= 0 && b < ext2fs.SuperBlock.S_blocks_count && !visitedBlock[b] {
				visitedBlock[b] = true
				nodeHierarchy[b] = &NodeInfo{ID: b, Level: depth + 1, Type: "folder_block"}
				levelNodes[depth+1] = append(levelNodes[depth+1], b)
				if err := processDirectoryEntriesForHierarchy(f, ext2fs, b, depth+2, visitedInode, visitedBlock); err != nil {
					fmt.Printf("WARN: procesando entradas del bloque %d: %v\n", b, err)
				}
			}
		}
	} else if inode.IsFile() {
		// Bloques directos (código existente)
		for i := 0; i < 12; i++ {
			b := inode.Iblock[i]
			if b >= 0 && b < ext2fs.SuperBlock.S_blocks_count && !visitedBlock[b] {
				visitedBlock[b] = true
				nodeHierarchy[b] = &NodeInfo{ID: b, Level: depth + 1, Type: "file_block"}
				levelNodes[depth+1] = append(levelNodes[depth+1], b)
			}
		}

		// *** NUEVO: Bloque indirecto simple (índice 12) ***
		if inode.Iblock[12] >= 0 && inode.Iblock[12] < ext2fs.SuperBlock.S_blocks_count && !visitedBlock[inode.Iblock[12]] {
			indirectBlock := inode.Iblock[12]
			visitedBlock[indirectBlock] = true
			nodeHierarchy[indirectBlock] = &NodeInfo{ID: indirectBlock, Level: depth + 1, Type: "pointer_block"}
			levelNodes[depth+1] = append(levelNodes[depth+1], indirectBlock)

			if err := processIndirectBlockForHierarchy(f, ext2fs, indirectBlock, depth+2, visitedBlock); err != nil {
				fmt.Printf("WARN: procesando bloque indirecto %d: %v\n", indirectBlock, err)
			}
		}

		// *** NUEVO: Bloque doble indirecto (índice 13) - si lo tienes ***
		if inode.Iblock[13] >= 0 && inode.Iblock[13] < ext2fs.SuperBlock.S_blocks_count && !visitedBlock[inode.Iblock[13]] {
			doubleIndirectBlock := inode.Iblock[13]
			visitedBlock[doubleIndirectBlock] = true
			nodeHierarchy[doubleIndirectBlock] = &NodeInfo{ID: doubleIndirectBlock, Level: depth + 1, Type: "pointer_block"}
			levelNodes[depth+1] = append(levelNodes[depth+1], doubleIndirectBlock)
		}
	}

	return nil
}

func processIndirectBlockForHierarchy(f *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, indirectBlockNum int32, depth int, visitedBlock map[int32]bool) error {
	offset := int64(ext2fs.SuperBlock.S_block_start + (indirectBlockNum * ext2fs.SuperBlock.S_block_s))
	buf := make([]byte, ext2fs.SuperBlock.S_block_s)
	if _, err := f.Seek(offset, 0); err != nil {
		return err
	}
	if _, err := f.Read(buf); err != nil {
		return err
	}

	// Usar tu estructura BloqueApuntador
	var bloqueApuntador bloques.BloqueApuntador
	if int32(len(buf)) >= bloqueApuntador.GetSize() {
		copy((*[64]byte)(unsafe.Pointer(&bloqueApuntador))[:], buf[:bloqueApuntador.GetSize()])
	}

	// Procesar los 16 apuntadores
	for i := 0; i < 16; i++ {
		blockNum := bloqueApuntador.B_pointers[i]
		if blockNum >= 0 && blockNum < ext2fs.SuperBlock.S_blocks_count && !visitedBlock[blockNum] {
			visitedBlock[blockNum] = true
			nodeHierarchy[blockNum] = &NodeInfo{ID: blockNum, Level: depth, Type: "file_block"}
			levelNodes[depth] = append(levelNodes[depth], blockNum)
		}
	}
	return nil
}

func processDirectoryEntriesForHierarchy(f *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, blockNum int32, depth int, visitedInode map[int32]bool, visitedBlock map[int32]bool) error {
	offset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
	buf := make([]byte, ext2fs.SuperBlock.S_block_s)
	if _, err := f.Seek(offset, 0); err != nil {
		return err
	}
	if _, err := f.Read(buf); err != nil {
		return err
	}

	var bc bloques.BloqueCarpeta
	if int32(len(buf)) >= bc.GetSize() {
		copy((*[64]byte)(unsafe.Pointer(&bc))[:], buf[:bc.GetSize()])
	}

	for _, entry := range bc.B_content {
		name := strings.TrimRight(string(entry.B_name[:]), "\x00")
		inodeRef := entry.B_inodo

		if name == "." || name == ".." || name == "" {
			continue
		}

		if inodeRef >= 0 && inodeRef < ext2fs.SuperBlock.S_inodes_count {
			buildHierarchy(f, ext2fs, inodeRef, depth, visitedInode, visitedBlock)
		}
	}
	return nil
}

// generateNodesAndEdges genera los nodos DOT y las conexiones de forma ordenada
func generateNodesAndEdges(f *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, inodeNum int32, dot *strings.Builder, visitedInode map[int32]bool, visitedBlock map[int32]bool, parentInode int32) error {
	if visitedInode[inodeNum] {
		return nil
	}
	visitedInode[inodeNum] = true

	inode, err := ext2fs.ReadInode(inodeNum)
	if err != nil {
		return fmt.Errorf("leer inodo %d: %v", inodeNum, err)
	}

	// Generar nodo inodo
	generateCleanInodeNode(dot, inode, inodeNum)

	if inode.IsFile() {
		return processFileInodeClean(f, ext2fs, inode, inodeNum, dot, visitedBlock)
	} else if inode.IsDirectory() {
		return processDirectoryInodeClean(f, ext2fs, inode, inodeNum, dot, visitedInode, visitedBlock)
	}

	return nil
}

func generateCleanInodeNode(dot *strings.Builder, inode *estructuraArchivoCarpeta.Inodo, inodeNum int32) {
	// Obtener la fecha actual
	currentTime := time.Now().Format("2006-01-02 15:04:05")

	// Determinar el tipo (1 para carpeta, 0 para archivo)
	typeValue := 0
	bgColor := "#98FB98" // Color para archivos
	if inode.IsDirectory() {
		typeValue = 1
		bgColor = "#87CEEB" // Color para carpetas
	}

	// Construir las filas de la tabla
	var rows strings.Builder
	rows.WriteString(fmt.Sprintf(`<TR><TD COLSPAN="2" BGCOLOR="#2E86C1"><FONT COLOR="white"><B>Inodo %d</B></FONT></TD></TR>`, inodeNum))
	rows.WriteString(fmt.Sprintf(`<TR><TD BGCOLOR="#AED6F1">ID</TD><TD BGCOLOR="white">%d</TD></TR>`, inodeNum))
	rows.WriteString(fmt.Sprintf(`<TR><TD BGCOLOR="#AED6F1">UID</TD><TD BGCOLOR="white">202307820</TD></TR>`))
	rows.WriteString(fmt.Sprintf(`<TR><TD BGCOLOR="#AED6F1">Fecha</TD><TD BGCOLOR="white">%s</TD></TR>`, currentTime))
	rows.WriteString(fmt.Sprintf(`<TR><TD BGCOLOR="#AED6F1">Tipo</TD><TD BGCOLOR="white">%d</TD></TR>`, typeValue))
	rows.WriteString(fmt.Sprintf(`<TR><TD BGCOLOR="#AED6F1">Size</TD><TD BGCOLOR="white">%d</TD></TR>`, inode.Is))

	// Agregar apuntadores directos (AD) - Limitar a un máximo de 3
	maxAD := 3
	for i := 0; i < 12; i++ {
		if inode.Iblock[i] >= 0 && i+1 > maxAD {
			maxAD = i + 1
		}
	}
	for i := 0; i < maxAD; i++ {
		if inode.Iblock[i] >= 0 {
			rows.WriteString(fmt.Sprintf(`<TR><TD BGCOLOR="#AED6F1">AD_%d</TD><TD BGCOLOR="white">%d</TD></TR>`, i+1, inode.Iblock[i]))
		} else {
			rows.WriteString(fmt.Sprintf(`<TR><TD BGCOLOR="#AED6F1">AD_%d</TD><TD BGCOLOR="white">-1</TD></TR>`, i+1))
		}
	}

	// Agregar apuntadores indirectos (AI)
	for i := 12; i < 15; i++ {
		if inode.Iblock[i] >= 0 {
			rows.WriteString(fmt.Sprintf(`<TR><TD BGCOLOR="#AED6F1">AI_%d</TD><TD BGCOLOR="white">%d</TD></TR>`, i-11, inode.Iblock[i]))
		} else {
			rows.WriteString(fmt.Sprintf(`<TR><TD BGCOLOR="#AED6F1">AI_%d</TD><TD BGCOLOR="white">-1</TD></TR>`, i-11))
		}
	}

	// Crear la tabla con los datos del inodo
	label := fmt.Sprintf(`< <TABLE BORDER="2" CELLBORDER="1" CELLSPACING="0" CELLPADDING="6" BGCOLOR="%s">%s</TABLE> >`, bgColor, rows.String())
	dot.WriteString(fmt.Sprintf("  Inodo%d [label=%s];\n", inodeNum, label))
}

func processFileInodeClean(f *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, inode *estructuraArchivoCarpeta.Inodo, inodeNum int32, dot *strings.Builder, visitedBlock map[int32]bool) error {
	// Bloques directos (código existente)
	for i := 0; i < 12; i++ {
		b := inode.Iblock[i]
		if b >= 0 && b < ext2fs.SuperBlock.S_blocks_count && !visitedBlock[b] {
			visitedBlock[b] = true
			generateCleanFileBlockNode(f, ext2fs, b, dot)
			dot.WriteString(fmt.Sprintf("  Inodo%d -> Bloque%d [color=\"#4A90E2\"];\n", inodeNum, b))
		}
	}

	// *** NUEVO: Bloque indirecto simple (índice 12) ***
	if inode.Iblock[12] >= 0 && inode.Iblock[12] < ext2fs.SuperBlock.S_blocks_count && !visitedBlock[inode.Iblock[12]] {
		indirectBlock := inode.Iblock[12]
		visitedBlock[indirectBlock] = true

		// Generar nodo del bloque apuntador
		generateIndirectBlockNode(f, ext2fs, indirectBlock, dot)
		dot.WriteString(fmt.Sprintf("  Inodo%d -> Bloque%d [color=\"#FF6B6B\", label=\"indirecto\"];\n", inodeNum, indirectBlock))

		// Procesar los bloques apuntados
		if err := processIndirectBlockContent(f, ext2fs, indirectBlock, dot, visitedBlock); err != nil {
			fmt.Printf("WARN: procesando contenido bloque indirecto %d: %v\n", indirectBlock, err)
		}
	}

	return nil
}

func generateIndirectBlockNode(f *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, blockNum int32, dot *strings.Builder) error {
	offset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
	buf := make([]byte, ext2fs.SuperBlock.S_block_s)
	if _, err := f.Seek(offset, 0); err != nil {
		return err
	}
	if _, err := f.Read(buf); err != nil {
		return err
	}

	// Usar tu estructura BloqueApuntador
	var bloqueApuntador bloques.BloqueApuntador
	if int32(len(buf)) >= bloqueApuntador.GetSize() {
		copy((*[64]byte)(unsafe.Pointer(&bloqueApuntador))[:], buf[:bloqueApuntador.GetSize()])
	}

	// Construir tabla con los apuntadores
	label := fmt.Sprintf(`< <TABLE BORDER="2" CELLBORDER="1" CELLSPACING="0" CELLPADDING="6" BGCOLOR="#FFB6C1">
        <TR><TD COLSPAN="2" BGCOLOR="#DC143C"><FONT COLOR="white"><B>Bloque Apuntador %d</B></FONT></TD></TR>`, blockNum)

	// Mostrar solo los apuntadores que no sean -1 para mantener limpio
	validPointers := 0
	for i := 0; i < 16; i++ {
		if bloqueApuntador.B_pointers[i] >= 0 {
			label += fmt.Sprintf(`<TR><TD BGCOLOR="white">P%d</TD><TD BGCOLOR="white">%d</TD></TR>`, i, bloqueApuntador.B_pointers[i])
			validPointers++
		}
	}

	// Si no hay apuntadores válidos, mostrar que está vacío
	if validPointers == 0 {
		label += `<TR><TD COLSPAN="2" BGCOLOR="white">(vacío)</TD></TR>`
	}

	label += "</TABLE> >"
	dot.WriteString(fmt.Sprintf("  Bloque%d [label=%s];\n", blockNum, label))
	return nil
}

func processIndirectBlockContent(f *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, indirectBlockNum int32, dot *strings.Builder, visitedBlock map[int32]bool) error {
	offset := int64(ext2fs.SuperBlock.S_block_start + (indirectBlockNum * ext2fs.SuperBlock.S_block_s))
	buf := make([]byte, ext2fs.SuperBlock.S_block_s)
	if _, err := f.Seek(offset, 0); err != nil {
		return err
	}
	if _, err := f.Read(buf); err != nil {
		return err
	}

	// Usar tu estructura BloqueApuntador
	var bloqueApuntador bloques.BloqueApuntador
	if int32(len(buf)) >= bloqueApuntador.GetSize() {
		copy((*[64]byte)(unsafe.Pointer(&bloqueApuntador))[:], buf[:bloqueApuntador.GetSize()])
	}

	// Procesar cada uno de los 16 apuntadores
	for i := 0; i < 16; i++ {
		blockNum := bloqueApuntador.B_pointers[i]
		if blockNum >= 0 && blockNum < ext2fs.SuperBlock.S_blocks_count && !visitedBlock[blockNum] {
			visitedBlock[blockNum] = true

			// Generar el bloque de datos
			generateCleanFileBlockNode(f, ext2fs, blockNum, dot)

			// Crear conexión del bloque apuntador al bloque de datos
			dot.WriteString(fmt.Sprintf("  Bloque%d -> Bloque%d [color=\"#4A90E2\"];\n", indirectBlockNum, blockNum))
		}
	}
	return nil
}

func processDirectoryInodeClean(f *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, inode *estructuraArchivoCarpeta.Inodo, inodeNum int32, dot *strings.Builder, visitedInode map[int32]bool, visitedBlock map[int32]bool) error {
	// Procesar solo bloques directos de carpeta
	for i := 0; i < 12; i++ {
		b := inode.Iblock[i]
		if b >= 0 && b < ext2fs.SuperBlock.S_blocks_count && !visitedBlock[b] {
			visitedBlock[b] = true

			// Conexión de inodo a bloque
			dot.WriteString(fmt.Sprintf("  Inodo%d -> Bloque%d [color=\"#4A90E2\"];\n", inodeNum, b))

			// Generar bloque y procesar entradas
			if err := generateCleanFolderBlockAndRecurse(f, ext2fs, b, dot, visitedInode, visitedBlock, b, inodeNum); err != nil {
				fmt.Printf("WARN: procesando bloque carpeta %d: %v\n", b, err)
			}
		}
	}
	return nil
}

func generateCleanFileBlockNode(f *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, blockNum int32, dot *strings.Builder) error {
	offset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
	buf := make([]byte, ext2fs.SuperBlock.S_block_s)
	if _, err := f.Seek(offset, 0); err != nil {
		return err
	}
	if _, err := f.Read(buf); err != nil {
		return err
	}

	var ba bloques.BloqueArchivo
	if int32(len(buf)) >= ba.GetSize() {
		copy((*[64]byte)(unsafe.Pointer(&ba))[:], buf[:ba.GetSize()])
	}

	content := ba.GetContent()
	r := []rune(content)
	if len(r) > fileCap {
		r = r[:fileCap]
		content = string(r) + "..."
	}

	// Escapar caracteres especiales
	content = strings.ReplaceAll(content, "&", "&amp;")
	content = strings.ReplaceAll(content, "<", "&lt;")
	content = strings.ReplaceAll(content, ">", "&gt;")
	content = strings.ReplaceAll(content, "\n", "<BR/>")
	content = strings.ReplaceAll(content, "\"", "&quot;")

	if content == "" {
		content = "(vacío)"
	}

	label := fmt.Sprintf(`< <TABLE BORDER="2" CELLBORDER="0" CELLSPACING="0" CELLPADDING="6" BGCOLOR="#FFFACD">
		<TR><TD BGCOLOR="#DAA520"><FONT COLOR="white"><B>Bloque Archivo %d</B></FONT></TD></TR>
		<TR><TD BGCOLOR="white" WIDTH="150">%s</TD></TR>
		</TABLE> >`, blockNum, content)

	dot.WriteString(fmt.Sprintf("  Bloque%d [label=%s];\n", blockNum, label))
	return nil
}

func generateCleanFolderBlockAndRecurse(f *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, blockNum int32, dot *strings.Builder, visitedInode map[int32]bool, visitedBlock map[int32]bool, inodeNum int32, parentInodeNum int32) error {
	offset := int64(ext2fs.SuperBlock.S_block_start + (blockNum * ext2fs.SuperBlock.S_block_s))
	buf := make([]byte, ext2fs.SuperBlock.S_block_s)
	if _, err := f.Seek(offset, 0); err != nil {
		return err
	}
	if _, err := f.Read(buf); err != nil {
		return err
	}

	var bc bloques.BloqueCarpeta
	if int32(len(buf)) >= bc.GetSize() {
		copy((*[64]byte)(unsafe.Pointer(&bc))[:], buf[:bc.GetSize()])
	}

	// Generar nodo bloque carpeta
	label := fmt.Sprintf(`< <TABLE BORDER="2" CELLBORDER="1" CELLSPACING="0" CELLPADDING="6" BGCOLOR="#FFE4E1">
        <TR><TD COLSPAN="2" BGCOLOR="#CD5C5C"><FONT COLOR="white"><B>Bloque Carpeta %d</B></FONT></TD></TR>`, blockNum)

	// Agregar la entrada `.` (inodo actual)
	label += fmt.Sprintf(`<TR><TD BGCOLOR="white">.</TD><TD BGCOLOR="white">%d</TD></TR>`, inodeNum)

	// Agregar la entrada `..` (inodo padre)
	label += fmt.Sprintf(`<TR><TD BGCOLOR="white">..</TD><TD BGCOLOR="white">%d</TD></TR>`, parentInodeNum)

	var validEntries []bloques.B_content

	// Procesar las entradas restantes
	for _, entry := range bc.B_content {
		name := strings.TrimRight(string(entry.B_name[:]), "\x00")
		if name == "" || name == "." || name == ".." {
			continue
		}
		validEntries = append(validEntries, entry)
	}

	// Mostrar entradas válidas
	for _, entry := range validEntries {
		name := strings.TrimRight(string(entry.B_name[:]), "\x00")
		label += fmt.Sprintf(`<TR><TD BGCOLOR="white">%s</TD><TD BGCOLOR="white">%d</TD></TR>`, name, entry.B_inodo)
	}

	label += "</TABLE> >"
	dot.WriteString(fmt.Sprintf("  Bloque%d [label=%s];\n", blockNum, label))

	// Crear conexiones y recursar para entradas válidas
	for _, entry := range validEntries {
		inodeRef := entry.B_inodo
		if inodeRef >= 0 && inodeRef < ext2fs.SuperBlock.S_inodes_count {
			dot.WriteString(fmt.Sprintf("  Bloque%d -> Inodo%d;\n", blockNum, inodeRef))
			if !visitedInode[inodeRef] {
				// Pasar el inodo actual como el nuevo padre
				generateNodesAndEdges(f, ext2fs, inodeRef, dot, visitedInode, visitedBlock, inodeNum)
			}
		}
	}

	return nil
}

func generateImprovedLevelConstraints(dot *strings.Builder) {
	// Ordenar niveles
	var levels []int
	for level := range levelNodes {
		levels = append(levels, level)
	}
	sort.Ints(levels)

	// Crear subgrafos invisibles para organizar por niveles
	for i, level := range levels {
		if len(levelNodes[level]) > 0 {
			dot.WriteString(fmt.Sprintf("  // Nivel %d\n", level))
			dot.WriteString(fmt.Sprintf("  subgraph cluster_%d {\n", i))
			dot.WriteString("    style=invis;\n")
			dot.WriteString("    rankdir=LR;\n") // Organizar horizontalmente dentro del nivel

			// Agregar nodos del mismo nivel
			for _, nodeID := range levelNodes[level] {
				if nodeInfo, exists := nodeHierarchy[nodeID]; exists {
					if nodeInfo.Type == "inode" {
						dot.WriteString(fmt.Sprintf("    Inodo%d;\n", nodeID))
					}
				}
			}
			dot.WriteString("  }\n")
		}
	}

	// Constraints de ranking para mantener jerarquía
	for _, level := range levels {
		nodes := levelNodes[level]
		if len(nodes) > 1 {
			var inodeNames []string
			for _, nodeID := range nodes {
				if nodeInfo, exists := nodeHierarchy[nodeID]; exists && nodeInfo.Type == "inode" {
					inodeNames = append(inodeNames, fmt.Sprintf("Inodo%d", nodeID))
				}
			}
			if len(inodeNames) > 1 {
				dot.WriteString(fmt.Sprintf("  { rank = same; %s; }\n", strings.Join(inodeNames, "; ")))
			}
		}
	}

	// Forzar orden jerárquico
	for i := 0; i < len(levels)-1; i++ {
		currentLevel := levels[i]
		nextLevel := levels[i+1]

		currentNodes := levelNodes[currentLevel]
		nextNodes := levelNodes[nextLevel]

		if len(currentNodes) > 0 && len(nextNodes) > 0 {
			// Crear una conexión invisible para forzar orden
			var currentInodes []string
			var nextInodes []string

			for _, nodeID := range currentNodes {
				if nodeInfo, exists := nodeHierarchy[nodeID]; exists && nodeInfo.Type == "inode" {
					currentInodes = append(currentInodes, fmt.Sprintf("Inodo%d", nodeID))
				}
			}

			for _, nodeID := range nextNodes {
				if nodeInfo, exists := nodeHierarchy[nodeID]; exists && nodeInfo.Type == "inode" {
					nextInodes = append(nextInodes, fmt.Sprintf("Inodo%d", nodeID))
				}
			}
		}
	}
}
