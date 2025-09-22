package report

import (
	bloques "MIA_2S2025_P1_202307820/Bloques"
	EstructuraExt2 "MIA_2S2025_P1_202307820/EstructuraExt2"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

// GenerateLsReport genera un reporte gráfico (imagen) que lista los archivos y carpetas
// contenidos en dirPath dentro del EXT2. Muestra permisos, owner (nombre si existe en users.txt),
// grupo (nombre si existe), size, fecha/hora modificación, tipo y nombre.
func GenerateLsReport(diskPath string, partitionStart int32, dirPath string, outputPath string) error {
	ext2fs, err := EstructuraExt2.ReadEXT2FileSystem(diskPath, partitionStart)
	if err != nil {
		return fmt.Errorf("leer EXT2: %v", err)
	}

	// abrir disco
	f, err := os.OpenFile(diskPath, os.O_RDONLY, 0666)
	if err != nil {
		return fmt.Errorf("abrir disco: %v", err)
	}
	defer f.Close()

	// resolver inodo del directorio
	dirInodeIdx, err := resolvePathToInode(ext2fs, f, dirPath)
	if err != nil {
		return fmt.Errorf("resolver ruta '%s': %v", dirPath, err)
	}

	dirInode, err := ext2fs.ReadInode(dirInodeIdx)
	if err != nil {
		return fmt.Errorf("leer inodo %d: %v", dirInodeIdx, err)
	}
	if !dirInode.IsDirectory() {
		return fmt.Errorf("ruta '%s' no es un directorio", dirPath)
	}

	// intentar leer users.txt para mapear UID/GID a nombres
	usersMap, groupsMap := parseUsersFile(ext2fs)

	// construir DOT
	var dot strings.Builder
	title := fmt.Sprintf("Reporte LS - %s", dirPath)
	dot.WriteString("digraph LS_REPORT {\n")
	dot.WriteString("  node [shape=plaintext];\n")
	dot.WriteString(fmt.Sprintf("  labelloc=\"t\"; label=\"%s\";\n\n", title))

	dot.WriteString("  ls_table [label=<\n")
	dot.WriteString("  <TABLE BORDER=\"1\" CELLBORDER=\"1\" CELLSPACING=\"0\" CELLPADDING=\"6\">\n")
	// encabezado
	dot.WriteString("    <TR BGCOLOR=\"#336699\"><TD><FONT COLOR=\"black\"><B>Permisos</B></FONT></TD><TD><FONT COLOR=\"black\"><B>Owner</B></FONT></TD><TD><FONT COLOR=\"black\"><B>Grupo</B></FONT></TD><TD><FONT COLOR=\"black\"><B>Size (Bytes)</B></FONT></TD><TD><FONT COLOR=\"black\"><B>Fecha Modif</B></FONT></TD><TD><FONT COLOR=\"black\"><B>Hora Modif</B></FONT></TD><TD><FONT COLOR=\"black\"><B>Tipo</B></FONT></TD><TD><FONT COLOR=\"black\"><B>Name</B></FONT></TD><TD><FONT COLOR=\"black\"><B>Fecha Creacion</B></FONT></TD></TR>\n")

	// recorrer bloques del directorio (directos 0..11)
	for i := 0; i < 12; i++ {
		bn := dirInode.Iblock[i]
		if bn < 0 || bn >= ext2fs.SuperBlock.S_blocks_count {
			continue
		}
		blockEntries, err := readFolderBlock(f, ext2fs, bn)
		if err != nil {
			continue
		}
		for _, entry := range blockEntries {
			name := strings.TrimRight(string(entry.B_name[:]), "\x00")
			if name == "" {
				continue
			}
			// no mostrar "." y ".."
			if name == "." || name == ".." {
				continue
			}
			inodeIdx := entry.B_inodo
			in, err := ext2fs.ReadInode(inodeIdx)
			if err != nil {
				continue
			}
			perm := in.GetPermissionsString()
			owner := strconv.Itoa(int(in.GetIuid()))
			if u, ok := usersMap[in.GetIuid()]; ok && u.Username != "" {
				owner = u.Username
			}
			group := strconv.Itoa(int(in.GetIgid()))
			if g, ok := groupsMap[in.GetIgid()]; ok && g != "" {
				group = g
			}
			size := in.GetIs()

			modTime := time.Unix(in.Imtime, 0)
			dateMod := modTime.Format("2006-01-02")
			hourMod := modTime.Format("15:04:05")

			creation := time.Unix(in.Ictime, 0).Format("2006-01-02 15:04:05")

			typ := "Archivo"
			if in.IsDirectory() {
				typ = "Carpeta"
			}

			// fila
			dot.WriteString(fmt.Sprintf("    <TR><TD>%s</TD><TD>%s</TD><TD>%s</TD><TD>%d</TD><TD>%s</TD><TD>%s</TD><TD>%s</TD><TD>%s</TD><TD>%s</TD></TR>\n",
				perm, owner, group, size, dateMod, hourMod, typ, escapeHTML(name), creation))
		}
	}

	dot.WriteString("  </TABLE>\n")
	dot.WriteString("  >];\n")
	dot.WriteString("}\n")

	// escribir dot a disco y generar imagen
	dotFile := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".dot"
	if err := os.WriteFile(dotFile, []byte(dot.String()), 0644); err != nil {
		return fmt.Errorf("escribir dot: %v", err)
	}

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

// helper: escape HTML básica para celdas
func escapeHTML(s string) string {
	// Primero reemplazar caracteres especiales en HTML
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")

	// Luego filtrar caracteres no imprimibles que podrían causar problemas
	var result strings.Builder
	for _, r := range s {
		if r >= 32 && r <= 126 || r == '\n' || r == '\r' || r == '\t' {
			// Caracteres ASCII imprimibles y algunos caracteres de control comunes
			result.WriteRune(r)
		} else {
			// Reemplazar otros caracteres con un punto
			result.WriteRune('.')
		}
	}
	return result.String()
}

// readFolderBlock lee un BloqueCarpeta desde disco y retorna sus entradas no vacías
func readFolderBlock(f *os.File, fs *EstructuraExt2.EXT2FileSystem, blockNum int32) ([]bloques.B_content, error) {
	offset := int64(fs.SuperBlock.S_block_start) + int64(blockNum)*int64(fs.SuperBlock.S_block_s)
	buf := make([]byte, fs.SuperBlock.S_block_s)
	if _, err := f.ReadAt(buf, offset); err != nil {
		return nil, err
	}
	var bc bloques.BloqueCarpeta
	if int32(len(buf)) >= bc.GetSize() {
		copy((*[64]byte)(unsafe.Pointer(&bc))[:], buf[:bc.GetSize()])
	} else {
		copy((*[64]byte)(unsafe.Pointer(&bc))[:], buf)
	}
	var out []bloques.B_content
	for _, e := range bc.B_content {
		if e.B_inodo != -1 {
			out = append(out, e)
		}
	}
	return out, nil
}

// resolvePathToInode resuelve una ruta absoluta dentro del FS y retorna el número de inodo
func resolvePathToInode(fs *EstructuraExt2.EXT2FileSystem, f *os.File, p string) (int32, error) {
	if p == "/" || p == "" {
		return 0, nil
	}
	parts := strings.Split(strings.Trim(p, "/"), "/")
	cur := int32(0) // raíz
	for _, comp := range parts {
		if comp == "" {
			continue
		}
		found := int32(-1)
		// leer inodo cur
		in, err := fs.ReadInode(cur)
		if err != nil {
			return -1, err
		}
		// buscar en directos
		for i := 0; i < 12 && found == -1; i++ {
			b := in.Iblock[i]
			if b < 0 || b >= fs.SuperBlock.S_blocks_count {
				continue
			}
			entries, err := readFolderBlock(f, fs, b)
			if err != nil {
				continue
			}
			for _, e := range entries {
				name := strings.TrimRight(string(e.B_name[:]), "\x00")
				if name == comp {
					found = e.B_inodo
					break
				}
			}
		}
		// indirecto simple si no encontrado
		if found == -1 && in.Iblock[12] >= 0 {
			// leer apuntador
			ptrBlock := in.Iblock[12]
			ptrOff := int64(fs.SuperBlock.S_block_start) + int64(ptrBlock)*int64(fs.SuperBlock.S_block_s)
			ptrBuf := make([]byte, fs.SuperBlock.S_block_s)
			if _, err := f.ReadAt(ptrBuf, ptrOff); err == nil {
				var bap bloques.BloqueApuntador
				if int32(len(ptrBuf)) >= bap.GetSize() {
					copy((*[64]byte)(unsafe.Pointer(&bap))[:], ptrBuf[:bap.GetSize()])
				} else {
					copy((*[64]byte)(unsafe.Pointer(&bap))[:], ptrBuf)
				}
				for _, ptr := range bap.B_pointers {
					if ptr < 0 || ptr >= fs.SuperBlock.S_blocks_count {
						continue
					}
					entries, err := readFolderBlock(f, fs, ptr)
					if err != nil {
						continue
					}
					for _, e := range entries {
						name := strings.TrimRight(string(e.B_name[:]), "\x00")
						if name == comp {
							found = e.B_inodo
							break
						}
					}
					if found != -1 {
						break
					}
				}
			}
		}
		if found == -1 {
			return -1, fmt.Errorf("componente no encontrado: %s", comp)
		}
		cur = found
	}
	return cur, nil
}

// user/group parsing

type usrEntry struct {
	ID       int32
	Kind     string // "G" o "U"
	Username string
	Group    string // para usuarios: grupo nombre
	Password string
}

// parseUsersFile intenta leer /users.txt y devuelve mapas: uid->username, gid->groupname
func parseUsersFile(fs *EstructuraExt2.EXT2FileSystem) (map[int32]usrEntry, map[int32]string) {
	users := make(map[int32]usrEntry)
	groups := make(map[int32]string)

	// ext2fs.ReadFile está disponible y devuelve contenido textual
	content, err := fs.ReadFile("/users.txt")
	if err != nil || strings.TrimSpace(content) == "" {
		return users, groups
	}
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		id, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		if parts[1] == "G" && len(parts) >= 3 {
			groups[int32(id)] = parts[2]
			users[int32(id)] = usrEntry{ID: int32(id), Kind: "G", Username: "", Group: parts[2], Password: ""}
		} else if parts[1] == "U" && len(parts) >= 5 {
			uid := int32(id)
			users[uid] = usrEntry{ID: uid, Kind: "U", Username: parts[2], Group: parts[3], Password: parts[4]}
		}
	}
	return users, groups
}
