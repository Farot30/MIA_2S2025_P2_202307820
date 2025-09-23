package reportP2

import (
	EstructuraExt2P2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	estructuraArchivoCarpetaP2 "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GenerateSuperblockReport genera un reporte (imagen) con la información del superbloque.
// diskPath: ruta al archivo disco (.dsk)

// outputPath: ruta de salida (ej: /home/user/reports/report8.jpg)
func GenerateSuperblockReport(diskPath string, partitionStart int32, outputPath string) error {
	ext2fs, err := EstructuraExt2P2.ReadEXT2FileSystem(diskPath, partitionStart)
	if err != nil {
		return fmt.Errorf("leer EXT2: %v", err)
	}

	// validar magic (opcional)
	if ext2fs.SuperBlock.S_magic != estructuraArchivoCarpetaP2.EXT2_MAGIC {
		// seguir pero avisar
		// return fmt.Errorf("no es EXT2 válido (magic mismatch)")
	}

	sb := ext2fs.SuperBlock

	var b strings.Builder
	b.WriteString("digraph SB_REPORT {\n")
	b.WriteString("  node [shape=plaintext];\n")
	b.WriteString("  labelloc=\"t\"; label=\"Reporte de SUPERBLOQUE\";\n\n")

	b.WriteString("  SuperBloque [label=<\n")
	b.WriteString("  <TABLE BORDER=\"1\" CELLBORDER=\"1\" CELLSPACING=\"0\" CELLPADDING=\"4\">\n")
	b.WriteString("    <TR><TD COLSPAN=\"2\" BGCOLOR=\"#0B6B3A\"><FONT COLOR=\"white\"><B>Reporte de SUPERBLOQUE</B></FONT></TD></TR>\n")

	diskName := filepath.Base(diskPath)
	b.WriteString(fmt.Sprintf("    <TR><TD>sb_nombre_hd</TD><TD>%v</TD></TR>\n", diskName))
	b.WriteString(fmt.Sprintf("    <TR><TD>sb_filesystem_type</TD><TD>%v</TD></TR>\n", sb.S_filesystem_type))
	b.WriteString(fmt.Sprintf("    <TR><TD>sb_inodes_count</TD><TD>%v</TD></TR>\n", sb.S_inodes_count))
	b.WriteString(fmt.Sprintf("    <TR><TD>sb_blocks_count</TD><TD>%v</TD></TR>\n", sb.S_blocks_count))

	b.WriteString(fmt.Sprintf("    <TR><TD>sb_free_blocks_count</TD><TD>%v</TD></TR>\n", sb.S_free_blocks_count))
	b.WriteString(fmt.Sprintf("    <TR><TD>sb_free_inodes_count</TD><TD>%v</TD></TR>\n", sb.S_free_inodes_count))

	// mostrar timestamps legibles
	mtime := time.Unix(sb.S_mtime, 0).Format("2006-01-02 15:04:05")
	umtime := time.Unix(sb.S_umtime, 0).Format("2006-01-02 15:04:05")
	b.WriteString(fmt.Sprintf("    <TR><TD>sb_mtime</TD><TD>%v</TD></TR>\n", mtime))
	b.WriteString(fmt.Sprintf("    <TR><TD>sb_umtime</TD><TD>%v</TD></TR>\n", umtime))
	b.WriteString(fmt.Sprintf("    <TR><TD>sb_mnt_count</TD><TD>%v</TD></TR>\n", sb.S_mnt_count))

	b.WriteString(fmt.Sprintf("    <TR><TD>sb_magic</TD><TD>%v</TD></TR>\n", sb.S_magic))

	b.WriteString(fmt.Sprintf("    <TR><TD>sb_inode_size</TD><TD>%v</TD></TR>\n", sb.S_inode_s))
	b.WriteString(fmt.Sprintf("    <TR><TD>sb_block_size</TD><TD>%v</TD></TR>\n", sb.S_block_s))

	b.WriteString(fmt.Sprintf("    <TR><TD>sb_first_free_inode</TD><TD>%v</TD></TR>\n", sb.S_firts_ino))
	b.WriteString(fmt.Sprintf("    <TR><TD>sb_first_free_block</TD><TD>%v</TD></TR>\n", sb.S_first_blo))

	b.WriteString(fmt.Sprintf("    <TR><TD>sb_bm_inode_start</TD><TD>%v</TD></TR>\n", sb.S_bm_inode_start))
	b.WriteString(fmt.Sprintf("    <TR><TD>sb_bm_block_start</TD><TD>%v</TD></TR>\n", sb.S_bm_block_start))
	b.WriteString(fmt.Sprintf("    <TR><TD>sb_inode_start</TD><TD>%v</TD></TR>\n", sb.S_inode_start))
	b.WriteString(fmt.Sprintf("    <TR><TD>sb_block_start</TD><TD>%v</TD></TR>\n", sb.S_block_start))

	b.WriteString("  </TABLE>\n")
	b.WriteString("  >];\n")
	b.WriteString("}\n")

	// escribir .dot
	dotFile := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".dot"
	if err := os.WriteFile(dotFile, []byte(b.String()), 0644); err != nil {
		return fmt.Errorf("escribir dot: %v", err)
	}

	// generar imagen via dot
	ext := filepath.Ext(outputPath)
	if ext == "" {
		outputPath += ".png"
		ext = ".png"
	}
	format := strings.TrimPrefix(ext, ".")
	cmd := exec.Command("dot", "-T"+format, dotFile, "-o", outputPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ejecutar dot: %v", err)
	}

	return nil
}
