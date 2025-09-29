package commandsP2

import (
	BloquesP2 "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt2P2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	EstructuraarchivocarpetaP2 "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"bytes"
	"fmt"
	"os"
	"strings"
)

// bytesToString convierte un array de bytes fijo a string, eliminando los bytes nulos
func bytesToString(b [12]byte) string {
	return string(bytes.Trim(b[:], "\x00"))
}

// GetBytesContent obtiene el contenido del bloque de archivo como bytes
func GetBytesContent(block *BloquesP2.BloqueArchivo, length int) []byte {
	if length <= 0 || length > 64 {
		length = 64
	}
	result := make([]byte, length)

	// Copiar byte por byte explícitamente para asegurar que no hay problemas de punteros
	for i := 0; i < length; i++ {
		result[i] = block.B_content[i]
	}

	// Debug para verificar el contenido que estamos extrayendo
	fmt.Printf("[Debug-GetBytes] Se extrajeron %d bytes del bloque\n", length)

	// Mostrar parte del contenido extraído para verificar
	contentStr := string(result)
	fmt.Printf("[Debug-GetBytes] Contenido extraído (hasta 30 caracteres): '%s'\n",
		strings.ReplaceAll(contentStr[:min(30, len(contentStr))], "\x00", "\\0"))

	return result
}

// SetBytesContent establece el contenido del bloque de archivo directamente como bytes
func SetBytesContent(block *BloquesP2.BloqueArchivo, content []byte) {
	// Primero limpiar completamente el array de destino
	for i := range block.B_content {
		block.B_content[i] = 0
	}

	// Copiar byte por byte explícitamente
	copyLen := len(content)
	if copyLen > 64 {
		copyLen = 64
	}

	for i := 0; i < copyLen; i++ {
		block.B_content[i] = content[i]
	}

	// Debug extensivo para verificar que se ha copiado correctamente
	fmt.Printf("[Debug-SetBytes] Se copiaron %d bytes al bloque\n", copyLen)

	// Imprimir el contenido como bytes para debugging
	fmt.Printf("[Debug-SetBytes] Primeros 10 bytes copiados: [")
	for i := 0; i < 10 && i < copyLen; i++ {
		if i > 0 {
			fmt.Printf(", ")
		}
		fmt.Printf("%d", block.B_content[i])
	}
	fmt.Printf("]\n")

	if copyLen > 0 {
		// Mostrar como texto para verificar contenido legible
		contentStr := string(block.B_content[:copyLen])
		fmt.Printf("[Debug-SetBytes] Contenido copiado (hasta 30 caracteres): '%s'\n",
			strings.ReplaceAll(contentStr[:min(30, len(contentStr))], "\x00", "\\0"))

		// Verificar si parece contenido de directorio
		if block.B_content[0] == '.' && (block.B_content[1] == 0 || block.B_content[1] == '.') {
			fmt.Printf("[ALERTA-SetBytes] Este bloque parece contener datos de directorio\n")
		}
	}
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// DebugBloqueArchivo imprime información de depuración sobre un bloque de archivo
func DebugBloqueArchivo(block *BloquesP2.BloqueArchivo, title string) {
	fmt.Printf("[Debug-Bloque] %s:\n", title)

	// Mostrar los primeros bytes como valores numéricos
	fmt.Printf("  - Bytes (20 primeros): [")
	for i := 0; i < 20 && i < len(block.B_content); i++ {
		if i > 0 {
			fmt.Printf(", ")
		}
		fmt.Printf("%d", block.B_content[i])
	}
	fmt.Printf("]\n")

	// Mostrar contenido como texto
	content := string(block.B_content[:])
	fmt.Printf("  - Como texto (hasta 20 caracteres): '%s'\n",
		strings.ReplaceAll(content[:min(20, len(content))], "\x00", "\\0"))
}

// findDirectoryInEXT2ForCopy busca un directorio en el sistema de archivos EXT2 y devuelve su número de inodo
func findDirectoryInEXT2ForCopy(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, path string) (int32, error) {
	// Caso especial: directorio raíz
	if path == "/" {
		return 0, nil // El inodo 0 siempre es la raíz
	}

	// Normalizar la ruta
	if !strings.HasPrefix(path, "/") {
		return -1, fmt.Errorf("la ruta debe ser absoluta (empezar con '/')")
	}

	// Eliminar el slash final si existe
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return 0, nil // Si después de eliminar el slash queda vacío, es la raíz
	}

	// Dividir la ruta en componentes
	components := strings.Split(path[1:], "/") // Ignorar el slash inicial
	currentDirInodeNum := int32(0)             // Comenzar desde el directorio raíz (inodo 0)

	// Recorrer cada componente de la ruta
	for _, component := range components {
		// Ignorar componentes vacíos
		if component == "" {
			continue
		}

		// Leer el inodo del directorio actual
		currentDirInode := &EstructuraarchivocarpetaP2.Inodo{}
		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+currentDirInodeNum*ext2fs.SuperBlock.S_inode_s), currentDirInode)
		if err != nil {
			return -1, fmt.Errorf("error al leer el inodo %d: %v", currentDirInodeNum, err)
		}

		// Verificar que sea un directorio
		if currentDirInode.Itype != 0 {
			return -1, fmt.Errorf("el inodo %d no es un directorio", currentDirInodeNum)
		}

		// Buscar el componente actual en el directorio
		found := false
		nextInodeNum := int32(-1)

		// Revisar los bloques directos del directorio
		for i := 0; i < 12 && !found; i++ {
			if currentDirInode.Iblock[i] == -1 {
				continue // Bloque no asignado
			}

			dirBlock := &BloquesP2.BloqueCarpeta{}
			err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+currentDirInode.Iblock[i]*ext2fs.SuperBlock.S_block_s), dirBlock)
			if err != nil {
				return -1, fmt.Errorf("error al leer el bloque de directorio %d: %v", currentDirInode.Iblock[i], err)
			}

			// Buscar la entrada en este bloque
			for j := 0; j < 4; j++ {
				entry := dirBlock.B_content[j]
				if entry.B_inodo != -1 && bytesToString(entry.B_name) == component {
					nextInodeNum = entry.B_inodo
					found = true
					break
				}
			}
		}

		// Si es necesario, buscar en bloques indirectos (simple, doble, triple)
		// Bloque indirecto simple (12)
		if !found && currentDirInode.Iblock[12] != -1 {
			ptrBlock := &BloquesP2.BloqueApuntador{}
			err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+currentDirInode.Iblock[12]*ext2fs.SuperBlock.S_block_s), ptrBlock)
			if err != nil {
				return -1, err
			}

			for i := 0; i < 16 && !found; i++ {
				if ptrBlock.B_pointers[i] == -1 {
					continue
				}

				dirBlock := &BloquesP2.BloqueCarpeta{}
				err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+ptrBlock.B_pointers[i]*ext2fs.SuperBlock.S_block_s), dirBlock)
				if err != nil {
					return -1, err
				}

				for j := 0; j < 4; j++ {
					entry := dirBlock.B_content[j]
					if entry.B_inodo != -1 && bytesToString(entry.B_name) == component {
						nextInodeNum = entry.B_inodo
						found = true
						break
					}
				}
			}
		}

		// Si no se encontró el componente, retornar error
		if !found {
			return -1, fmt.Errorf("no se encontró el directorio '%s' en la ruta", component)
		}

		// Actualizar el inodo actual para el siguiente nivel
		currentDirInodeNum = nextInodeNum
	}

	// Verificar que el inodo final sea un directorio
	finalInode := &EstructuraarchivocarpetaP2.Inodo{}
	err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+currentDirInodeNum*ext2fs.SuperBlock.S_inode_s), finalInode)
	if err != nil {
		return -1, fmt.Errorf("error al leer el inodo final %d: %v", currentDirInodeNum, err)
	}

	if finalInode.Itype != 0 {
		return -1, fmt.Errorf("la ruta '%s' no es un directorio", path)
	}

	return currentDirInodeNum, nil
}

// findFileInDirectoryEXT2ForCopy busca un archivo o directorio específico dentro de un directorio en EXT2
func findFileInDirectoryEXT2ForCopy(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, fileName string) (int32, error) {
	// Verificar que sea un directorio
	if dirInode.Itype != 0 {
		return -1, fmt.Errorf("el inodo proporcionado no es un directorio")
	}

	// Revisar los bloques directos del directorio
	for i := 0; i < 12; i++ {
		if dirInode.Iblock[i] == -1 {
			continue // Bloque no asignado
		}

		dirBlock := &BloquesP2.BloqueCarpeta{}
		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+dirInode.Iblock[i]*ext2fs.SuperBlock.S_block_s), dirBlock)
		if err != nil {
			return -1, fmt.Errorf("error al leer el bloque de directorio %d: %v", dirInode.Iblock[i], err)
		}

		// Buscar el archivo en este bloque
		for j := 0; j < 4; j++ {
			entry := dirBlock.B_content[j]
			if entry.B_inodo != -1 && bytesToString(entry.B_name) == fileName {
				return entry.B_inodo, nil
			}
		}
	}

	// Si es necesario, buscar en bloques indirectos (simple, doble, triple)
	// Bloque indirecto simple (12)
	if dirInode.Iblock[12] != -1 {
		ptrBlock := &BloquesP2.BloqueApuntador{}
		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+dirInode.Iblock[12]*ext2fs.SuperBlock.S_block_s), ptrBlock)
		if err != nil {
			return -1, err
		}

		for i := 0; i < 16; i++ {
			if ptrBlock.B_pointers[i] == -1 {
				continue
			}

			dirBlock := &BloquesP2.BloqueCarpeta{}
			err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+ptrBlock.B_pointers[i]*ext2fs.SuperBlock.S_block_s), dirBlock)
			if err != nil {
				return -1, err
			}

			for j := 0; j < 4; j++ {
				entry := dirBlock.B_content[j]
				if entry.B_inodo != -1 && bytesToString(entry.B_name) == fileName {
					return entry.B_inodo, nil
				}
			}
		}
	}

	// Si no se encontró el archivo, retornar -1 (no error, solo indica que no existe)
	return -1, nil
}

// findDirectoryInEXT3ForCopy busca un directorio en el sistema de archivos EXT3 y devuelve su número de inodo
func findDirectoryInEXT3ForCopy(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, path string) (int32, error) {
	// Caso especial: directorio raíz
	if path == "/" {
		return 0, nil // El inodo 0 siempre es la raíz
	}

	// Normalizar la ruta
	if !strings.HasPrefix(path, "/") {
		return -1, fmt.Errorf("la ruta debe ser absoluta (empezar con '/')")
	}

	// Eliminar el slash final si existe
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return 0, nil // Si después de eliminar el slash queda vacío, es la raíz
	}

	// Dividir la ruta en componentes
	components := strings.Split(path[1:], "/") // Ignorar el slash inicial
	currentDirInodeNum := int32(0)             // Comenzar desde el directorio raíz (inodo 0)

	// Recorrer cada componente de la ruta
	for _, component := range components {
		// Ignorar componentes vacíos
		if component == "" {
			continue
		}

		// Leer el inodo del directorio actual
		currentDirInode := &EstructuraarchivocarpetaP2.Inodo{}
		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+currentDirInodeNum*ext3fs.SuperBlock.S_inode_s), currentDirInode)
		if err != nil {
			return -1, fmt.Errorf("error al leer el inodo %d: %v", currentDirInodeNum, err)
		}

		// Verificar que sea un directorio
		if currentDirInode.Itype != 0 {
			return -1, fmt.Errorf("el inodo %d no es un directorio", currentDirInodeNum)
		}

		// Buscar el componente actual en el directorio
		found := false
		nextInodeNum := int32(-1)

		// Revisar los bloques directos del directorio
		for i := 0; i < 12 && !found; i++ {
			if currentDirInode.Iblock[i] == -1 {
				continue // Bloque no asignado
			}

			dirBlock := &BloquesP2.BloqueCarpeta{}
			err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+currentDirInode.Iblock[i]*ext3fs.SuperBlock.S_block_s), dirBlock)
			if err != nil {
				return -1, fmt.Errorf("error al leer el bloque de directorio %d: %v", currentDirInode.Iblock[i], err)
			}

			// Buscar la entrada en este bloque
			for j := 0; j < 4; j++ {
				entry := dirBlock.B_content[j]
				if entry.B_inodo != -1 && bytesToString(entry.B_name) == component {
					nextInodeNum = entry.B_inodo
					found = true
					break
				}
			}
		}

		// Si es necesario, buscar en bloques indirectos (simple, doble, triple)
		// Bloque indirecto simple (12)
		if !found && currentDirInode.Iblock[12] != -1 {
			ptrBlock := &BloquesP2.BloqueApuntador{}
			err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+currentDirInode.Iblock[12]*ext3fs.SuperBlock.S_block_s), ptrBlock)
			if err != nil {
				return -1, err
			}

			for i := 0; i < 16 && !found; i++ {
				if ptrBlock.B_pointers[i] == -1 {
					continue
				}

				dirBlock := &BloquesP2.BloqueCarpeta{}
				err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+ptrBlock.B_pointers[i]*ext3fs.SuperBlock.S_block_s), dirBlock)
				if err != nil {
					return -1, err
				}

				for j := 0; j < 4; j++ {
					entry := dirBlock.B_content[j]
					if entry.B_inodo != -1 && bytesToString(entry.B_name) == component {
						nextInodeNum = entry.B_inodo
						found = true
						break
					}
				}
			}
		}

		// Si no se encontró el componente, retornar error
		if !found {
			return -1, fmt.Errorf("no se encontró el directorio '%s' en la ruta", component)
		}

		// Actualizar el inodo actual para el siguiente nivel
		currentDirInodeNum = nextInodeNum
	}

	// Verificar que el inodo final sea un directorio
	finalInode := &EstructuraarchivocarpetaP2.Inodo{}
	err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+currentDirInodeNum*ext3fs.SuperBlock.S_inode_s), finalInode)
	if err != nil {
		return -1, fmt.Errorf("error al leer el inodo final %d: %v", currentDirInodeNum, err)
	}

	if finalInode.Itype != 0 {
		return -1, fmt.Errorf("la ruta '%s' no es un directorio", path)
	}

	return currentDirInodeNum, nil
}

// findFileInDirectoryEXT3ForCopy busca un archivo o directorio específico dentro de un directorio en EXT3
func findFileInDirectoryEXT3ForCopy(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, fileName string) (int32, error) {
	// Verificar que sea un directorio
	if dirInode.Itype != 0 {
		return -1, fmt.Errorf("el inodo proporcionado no es un directorio")
	}

	// Revisar los bloques directos del directorio
	for i := 0; i < 12; i++ {
		if dirInode.Iblock[i] == -1 {
			continue // Bloque no asignado
		}

		dirBlock := &BloquesP2.BloqueCarpeta{}
		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+dirInode.Iblock[i]*ext3fs.SuperBlock.S_block_s), dirBlock)
		if err != nil {
			return -1, fmt.Errorf("error al leer el bloque de directorio %d: %v", dirInode.Iblock[i], err)
		}

		// Buscar el archivo en este bloque
		for j := 0; j < 4; j++ {
			entry := dirBlock.B_content[j]
			if entry.B_inodo != -1 && bytesToString(entry.B_name) == fileName {
				return entry.B_inodo, nil
			}
		}
	}

	// Si es necesario, buscar en bloques indirectos (simple, doble, triple)
	// Bloque indirecto simple (12)
	if dirInode.Iblock[12] != -1 {
		ptrBlock := &BloquesP2.BloqueApuntador{}
		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+dirInode.Iblock[12]*ext3fs.SuperBlock.S_block_s), ptrBlock)
		if err != nil {
			return -1, err
		}

		for i := 0; i < 16; i++ {
			if ptrBlock.B_pointers[i] == -1 {
				continue
			}

			dirBlock := &BloquesP2.BloqueCarpeta{}
			err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+ptrBlock.B_pointers[i]*ext3fs.SuperBlock.S_block_s), dirBlock)
			if err != nil {
				return -1, err
			}

			for j := 0; j < 4; j++ {
				entry := dirBlock.B_content[j]
				if entry.B_inodo != -1 && bytesToString(entry.B_name) == fileName {
					return entry.B_inodo, nil
				}
			}
		}
	}

	// Si no se encontró el archivo, retornar -1 (no error, solo indica que no existe)
	return -1, nil
}
