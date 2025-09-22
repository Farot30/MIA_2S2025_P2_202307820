package report

import (
	EstructuraExt2 "MIA_2S2025_P1_202307820/EstructuraExt2"
	estructuraArchivoCarpeta "MIA_2S2025_P1_202307820/estructuraArchivoCarpeta"
	"MIA_2S2025_P1_202307820/utilities"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func GenerateInodeReport(diskPath string, partitionStart int32, outputPath string) error {
	// Abrir el archivo del disco
	file, err := os.OpenFile(diskPath, os.O_RDONLY, 0666)
	if err != nil {
		return fmt.Errorf("error abriendo el disco '%s': %s", diskPath, err)
	}
	defer file.Close()

	// Intentar leer el sistema de archivos EXT2
	ext2fs, err := EstructuraExt2.ReadEXT2FileSystem(diskPath, partitionStart)
	if err != nil {
		return fmt.Errorf("error leyendo sistema de archivos EXT2: %s", err)
	}

	// Verificar que sea un sistema de archivos EXT2 válido
	if ext2fs.SuperBlock.S_magic != estructuraArchivoCarpeta.EXT2_MAGIC {
		return fmt.Errorf("el sistema de archivos no es EXT2 válido (magic number incorrecto)")
	}

	// Generar código DOT para Graphviz
	var dot strings.Builder

	// Configurar el grafo para disposición horizontal mejorada
	dot.WriteString("digraph INODE_REPORT {\n")
	dot.WriteString("  graph [rankdir=LR, nodesep=0.8, ranksep=1.5, splines=ortho];\n")
	dot.WriteString("  node [shape=plaintext, fontname=\"Arial\"];\n")
	dot.WriteString("  edge [arrowhead=vee, arrowsize=0.8, color=\"#2E86C1\"];\n")
	dot.WriteString("  label=\"Reporte de Inodos Utilizados\";\n")
	dot.WriteString("  labelloc=\"t\";\n")
	dot.WriteString("  fontsize=16;\n")
	dot.WriteString("  fontname=\"Arial Bold\";\n\n")

	// Leer el bitmap de inodos
	bitmapInodeSize := (ext2fs.SuperBlock.S_inodes_count + 7) / 8
	bitmapInodeData := make([]byte, bitmapInodeSize)

	_, err = file.Seek(int64(ext2fs.SuperBlock.S_bm_inode_start), 0)
	if err != nil {
		return fmt.Errorf("error posicionándose en el bitmap de inodos: %s", err)
	}

	_, err = file.Read(bitmapInodeData)
	if err != nil {
		return fmt.Errorf("error leyendo el bitmap de inodos: %s", err)
	}

	// Lista para conservar el orden de inodos usados
	var usedInodes []int32

	// Para cada inodo
	for inodeNum := int32(0); inodeNum < ext2fs.SuperBlock.S_inodes_count; inodeNum++ {
		// Verificar si está en uso
		bytePos := inodeNum / 8
		bitPos := inodeNum % 8

		if bytePos < int32(len(bitmapInodeData)) {
			if (bitmapInodeData[bytePos] & (1 << (7 - bitPos))) != 0 {
				// El inodo está en uso, leer sus datos
				inode := &estructuraArchivoCarpeta.Inodo{}

				// Calcular posición del inodo en la tabla
				inodeOffset := ext2fs.SuperBlock.S_inode_start + (inodeNum * int32(ext2fs.SuperBlock.S_inode_s))

				// Leer el inodo
				err = utilities.ReadStruct(file, int64(inodeOffset), inode)
				if err != nil {
					return fmt.Errorf("error leyendo el inodo %d: %s", inodeNum, err)
				}

				// Generar representación gráfica del inodo
				generateInodeGraphNode(&dot, inode, inodeNum)

				// Añadir a la lista usada
				usedInodes = append(usedInodes, inodeNum)
			}
		}
	}

	// Crear enlaces entre los nodos para flujo horizontal
	// Sin forzar el mismo rank para permitir mejor distribución
	for i := 0; i < len(usedInodes)-1; i++ {
		src := usedInodes[i]
		dst := usedInodes[i+1]
		dot.WriteString(fmt.Sprintf("  Inodo%d -> Inodo%d;\n", src, dst))
	}

	dot.WriteString("}\n")

	// Escribir el archivo DOT
	dotFile := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".dot"
	if err := os.WriteFile(dotFile, []byte(dot.String()), 0644); err != nil {
		return fmt.Errorf("error escribiendo archivo DOT: %s", err)
	}

	// Generar la imagen con Graphviz
	ext := filepath.Ext(outputPath)
	if ext == "" {
		outputPath += ".png"
		ext = ".png"
	}

	// Eliminar el punto del ext
	format := strings.TrimPrefix(ext, ".")

	cmd := exec.Command("dot", "-T"+format, dotFile, "-o", outputPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error generando imagen con Graphviz: %s", err)
	}

	fmt.Printf("Reporte de inodos generado exitosamente: %s\n", outputPath)
	return nil
}

// generateInodeGraphNode genera un nodo gráfico para un inodo con mejor diseño
func generateInodeGraphNode(dot *strings.Builder, inode *estructuraArchivoCarpeta.Inodo, inodeNum int32) {
	// Convertir timestamp Unix a fecha legible
	atime := time.Unix(inode.Iatime, 0).Format("02/01/2006 15:04")

	// Mostrar solo hasta i_block_0..i_block_2, luego resumen si hay más
	maxShow := 3 // 0,1,2
	totalBlocks := len(inode.Iblock)
	var rows strings.Builder

	// Determinar color de fondo según el tipo
	headerColor := "#E8F8F5" // Verde claro para carpetas por defecto
	if inode.Itype == 1 {    // Archivo
		headerColor = "#EBF3FF" // Azul claro para archivos
	}

	// Cabecera del nodo como tabla HTML optimizada para horizontal
	rows.WriteString("<TABLE BORDER=\"1\" CELLBORDER=\"1\" CELLSPACING=\"0\" CELLPADDING=\"4\" BGCOLOR=\"white\">")
	rows.WriteString(fmt.Sprintf("<TR><TD COLSPAN=\"2\" BGCOLOR=\"%s\"><B>Inodo %d</B></TD></TR>", headerColor, inodeNum))

	// Tipo con icono visual
	typeStr := "Desconocido"

	switch inode.Itype {
	case 0:
		typeStr = "Carpeta"

	case 1:
		typeStr = "Archivo"
	}
	rows.WriteString(fmt.Sprintf("<TR><TD><B>Tipo</B></TD><TD> %s</TD></TR>", typeStr))

	// UID y tamaño con mejor formato
	rows.WriteString(fmt.Sprintf("<TR><TD><B>UID</B></TD><TD>%d</TD></TR>", inode.Iuid))
	rows.WriteString(fmt.Sprintf("<TR><TD><B>Size</B></TD><TD>%d bytes</TD></TR>", inode.Is))

	// Fecha acceso/tiempo
	rows.WriteString(fmt.Sprintf("<TR><TD><B>Atime</B></TD><TD>%s</TD></TR>", atime))

	// Separador visual para i_blocks
	rows.WriteString("<TR><TD COLSPAN=\"2\" BGCOLOR=\"#F8F9FA\"><B>Bloques de Datos</B></TD></TR>")

	// Mostrar i_blocks como filas hasta el límite
	for i := 0; i < totalBlocks && i < maxShow; i++ {
		blockValue := inode.Iblock[i]
		blockStatus := ""
		if blockValue == -1 {
			blockStatus = "-1"
		} else {
			blockStatus = fmt.Sprintf("%d", blockValue)
		}
		rows.WriteString(fmt.Sprintf("<TR><TD><B>i_block_%d</B></TD><TD>%s</TD></TR>", i, blockStatus))
	}

	if totalBlocks > maxShow {
		remaining := totalBlocks - maxShow
		rows.WriteString(fmt.Sprintf("<TR><TD COLSPAN=\"2\" BGCOLOR=\"#F0F0F0\"><I>... + %d i_blocks más</I></TD></TR>", remaining))
	}

	rows.WriteString("</TABLE>")

	// Etiqueta HTML completa
	label := fmt.Sprintf("<%s>", rows.String())
	dot.WriteString(fmt.Sprintf("  Inodo%d [label=%s];\n", inodeNum, label))
}
