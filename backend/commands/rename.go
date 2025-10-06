package commandsP2

import (
	bloques "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	sessionP2 "MIA_2S2025_P2_202307820/Session"
	EstructuraarchivocarpetaP2 "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func RENAME(args []string) {

	if !sessionP2.IsSessionActive() {
		fmt.Printf("Error: debe iniciar sesión para usar este comando\n")
		return
	}

	params := utilitiesP2.ParseParameters(args)
	path := strings.Trim(params["path"], "\"")
	newName := strings.Trim(params["name"], "\"")

	if path == "" || newName == "" {
		fmt.Printf("Error: faltan parámetros obligatorios para el comando RENAME\n")
		return
	}

	if !strings.HasPrefix(path, "/") {
		fmt.Printf("Error: el parámetro 'path' debe comenzar con '/'\n")
		return
	}

	sessionInfo := sessionP2.GetCurrentSession()
	if sessionInfo == nil {
		fmt.Printf("Error: no se pudo obtener información de la sesión actual\n")
		return
	}

	MountedPartition, exist := GetMountedPartition(sessionInfo.PartitionID)
	if !exist {
		fmt.Printf("Error: la partición no está montada\n")
		return
	}

	err := RenameFileOrDirectory(MountedPartition, path, newName, sessionInfo)
	if err != nil {
		fmt.Printf("Error: no se pudo renombrar el archivo o directorio\n")
		return
	}

	fmt.Printf("Archivo o directorio renombrado exitosamente\n")
}

func RenameFileOrDirectory(partition MountedPartition, path string, newName string, sessionInfo *sessionP2.UserSession) error {
	file, err := os.OpenFile(partition.Path, os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("no se pudo abrir el disco: %v", err)
	}
	defer file.Close()

	superBlock := &EstructuraarchivocarpetaP2.SuperBloque{}
	err = utilitiesP2.ReadStruct(file, int64(partition.Start), superBlock)
	if err != nil {
		return fmt.Errorf("error al leer el superbloque: %v", err)
	}

	fileName := filepath.Base(path)
	dirPath := filepath.Dir(path)
	if dirPath == "" {
		dirPath = "/"
	}

	var fileSystem int32
	switch superBlock.S_filesystem_type {
	case 2:
		fileSystem = 2
	case 3:
		fileSystem = 3
	default:
		return fmt.Errorf("sistema de archivos no soportado: %s", partition.Type)
	}

	switch fileSystem {
	case 2:
		ext2fs, err := EstructuraExt2.ReadEXT2FileSystem(partition.Path, partition.Start)
		if err != nil {
			return fmt.Errorf("error al leer el sistema EXT2: %v", err)
		}

		// Buscar el directorio padre
		parentInodeNum, err := findDirectoryInEXT2(file, ext2fs, dirPath)
		if err != nil {
			return fmt.Errorf("error al buscar el directorio padre: %v", err)
		}

		// Leer el inodo del directorio padre
		parentInode := &EstructuraarchivocarpetaP2.Inodo{}
		err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+parentInodeNum*ext2fs.SuperBlock.S_inode_s), parentInode)
		if err != nil {
			return fmt.Errorf("error al leer el inodo del directorio padre: %v", err)
		}

		// Buscar el archivo en el directorio
		fmt.Printf("Depuración: Buscando '%s' en el directorio padre\n", fileName)
		fileInodeNum, err := findFileInDirectoryEXT2(file, ext2fs, parentInode, fileName)
		if err != nil {
			return fmt.Errorf("error al buscar el archivo o carpeta: %v", err)
		}
		if fileInodeNum == -1 {
			return fmt.Errorf("el archivo o carpeta '%s' no existe", fileName)
		}

		// Leer el inodo del archivo o carpeta
		fileInode := &EstructuraarchivocarpetaP2.Inodo{}
		err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+fileInodeNum*ext2fs.SuperBlock.S_inode_s), fileInode)
		if err != nil {
			return fmt.Errorf("error al leer el inodo del archivo o carpeta: %v", err)
		}

		canRead, err := sessionP2.CheckPermissions(fileInode, sessionP2.READ)
		if err != nil || !canRead {
			return fmt.Errorf("el usuario no tiene permisos de lectura sobre el archivo")
		}

		canWrite, err := sessionP2.CheckPermissions(fileInode, sessionP2.WRITE)
		if err != nil || !canWrite {
			return fmt.Errorf("el usuario no tiene permisos de escritura sobre el archivo")
		}

		// Verificar el tipo del inodo (archivo o carpeta)
		switch fileInode.Itype {
		case 0:
			fmt.Printf("Depuración: Renombrando carpeta '%s'\n", fileName)
		case 1:
			fmt.Printf("Depuración: Renombrando archivo '%s'\n", fileName)
		default:
			return fmt.Errorf("tipo de inodo desconocido para '%s'", fileName)
		}

		fmt.Printf("Depuración: entrando a la función renombrar '%s'\n", fileName)

		// Actualizar el nombre en el bloque de directorio
		err = updateFileNameInDirectoryEXT2(file, ext2fs, parentInode, fileName, newName)
		if err != nil {
			return fmt.Errorf("error al renombrar el archivo o carpeta: %v", err)
		}

		// Escribir el inodo actualizado de vuelta al disco
		err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_inode_start+parentInodeNum*ext2fs.SuperBlock.S_inode_s), parentInode)
		if err != nil {
			return fmt.Errorf("error al actualizar el inodo del directorio padre: %v", err)
		}

		return nil

	case 3:
		ext3fs, err := EstructuraExt3P2.ReadEXT3FileSystem(partition.Path, partition.Start)
		if err != nil {
			return fmt.Errorf("error al leer el sistema EXT3: %v", err)
		}

		parentInodeNum, err := findDirectoryInEXT3(file, ext3fs, dirPath)
		if err != nil {
			return fmt.Errorf("error al buscar el directorio padre: %v", err)
		}

		parentInode := &EstructuraarchivocarpetaP2.Inodo{}
		err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+parentInodeNum*ext3fs.SuperBlock.S_inode_s), parentInodeNum)
		if err != nil {
			return fmt.Errorf("error al leer el inodo del directorio padre: %v", err)
		}

		fileInodeNum, err := findFileInDirectoryEXT3(file, ext3fs, parentInode, fileName)
		if err != nil {
			return fmt.Errorf("error al buscar el archivo: %v", err)
		}
		if fileInodeNum == -1 {
			return fmt.Errorf("el archivo '%s' no existe", fileName)
		}

		fileInode := &EstructuraarchivocarpetaP2.Inodo{}
		err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+fileInodeNum*ext3fs.SuperBlock.S_inode_s), fileInode)
		if err != nil {
			return fmt.Errorf("error al leer el inodo del archivo: %v", err)
		}

		if fileInode.Itype != 1 { // 1 = archivo
			return fmt.Errorf("'%s' no es un archivo", fileName)
		}

		canRead, err := sessionP2.CheckPermissions(fileInode, sessionP2.READ)
		if err != nil || !canRead {
			return fmt.Errorf("el usuario no tiene permisos de lectura sobre el archivo")
		}

		canWrite, err := sessionP2.CheckPermissions(fileInode, sessionP2.WRITE)
		if err != nil || !canWrite {
			return fmt.Errorf("el usuario no tiene permisos de escritura sobre el archivo")
		}

		err = updateFileNameInDirectoryEXT3(file, ext3fs, parentInode, fileName, newName)
		if err != nil {
			return fmt.Errorf("error al renombrar el archivo: %v", err)
		}

		err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_inode_start+parentInodeNum*ext3fs.SuperBlock.S_inode_s), parentInodeNum)
		if err != nil {
			return fmt.Errorf("error al actualizar el inodo del directorio padre: %v", err)
		}

		LogEXT3Operation(EstructuraExt3P2.OPERATION_RENAME, path, fmt.Sprintf("Renombrado a '%s'", newName))

		return nil
	}
	return fmt.Errorf("sistema de archivos no soportado")

}

func updateFileNameInDirectoryEXT2(file *os.File, ext2fs *EstructuraExt2.EXT2FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, oldName string, newName string) error {

	for i := 0; i < 12; i++ {
		if dirInode.Iblock[i] == -1 {
			continue
		}

		dirBlock := &bloques.BloqueCarpeta{}
		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+dirInode.Iblock[i]*ext2fs.SuperBlock.S_block_s), dirBlock)
		if err != nil {
			return fmt.Errorf("error al leer el bloque de directorio: %v", err)
		}

		// Buscar el nombre antiguo en el bloque
		found := false
		for j := 0; j < len(dirBlock.B_content); j++ {
			entry := &dirBlock.B_content[j]
			entryName := strings.TrimRight(string(entry.B_name[:]), "\x00")
			if entryName == oldName {
				found = true
				// Depuración: Nombre encontrado
				fmt.Printf("Depuración: Nombre '%s' encontrado en la entrada %d\n", oldName, j)

				// Limpiar el espacio del nombre anterior
				for k := range entry.B_name {
					entry.B_name[k] = 0
				}

				// Actualizar el nombre
				copy(entry.B_name[:], newName)

				// Depuración: Verificar el bloque después de actualizar
				fmt.Printf("Depuración: Bloque después de actualizar: %+v\n", dirBlock)

				// Escribir el bloque actualizado de vuelta al disco
				if err := utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+dirInode.Iblock[i]*ext2fs.SuperBlock.S_block_s), dirBlock); err != nil {
					return fmt.Errorf("error al escribir el bloque de directorio actualizado: %v", err)
				}

				// Depuración: Confirmar que el bloque fue escrito correctamente
				fmt.Printf("Depuración: Bloque de directorio actualizado escrito correctamente\n")

				return nil
			}
		}

		if !found {
			// Depuración: Nombre no encontrado en el bloque
			fmt.Printf("Depuración: Nombre '%s' no encontrado en el bloque\n", oldName)
		}
	}
	if dirInode.Iblock[12] != -1 {

		indirectBlock := make([]int32, ext2fs.SuperBlock.S_block_s/4)
		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+dirInode.Iblock[12]*ext2fs.SuperBlock.S_block_s), &indirectBlock)
		if err != nil {
			return fmt.Errorf("error al leer el bloque indirecto: %v", err)
		}

		for _, blockNum := range indirectBlock {
			if blockNum == -1 {
				continue
			}

			dirBlock := &bloques.BloqueCarpeta{}
			err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+blockNum*ext2fs.SuperBlock.S_block_s), dirBlock)
			if err != nil {
				return fmt.Errorf("error al leer el bloque de directorio: %v", err)
			}

			for j := 0; j < len(dirBlock.B_content); j++ {
				entry := &dirBlock.B_content[j]
				if string(entry.B_name[:]) == oldName {
					// Actualizar el nombre
					copy(entry.B_name[:], newName)

					// Escribir el bloque actualizado de vuelta al disco
					err := utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+blockNum*ext2fs.SuperBlock.S_block_s), dirBlock)
					if err != nil {
						return fmt.Errorf("error al escribir el bloque de directorio actualizado: %v", err)
					}

					return nil
				}
			}
		}
	}

	return fmt.Errorf("archivo '%s' no encontrado en el directorio", oldName)

}

func updateFileNameInDirectoryEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, oldName string, newName string) error {

	for i := 0; i < 12; i++ {
		if dirInode.Iblock[i] == -1 {
			continue
		}

		dirBlock := &bloques.BloqueCarpeta{}
		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+dirInode.Iblock[i]*ext3fs.SuperBlock.S_block_s), dirBlock)
		if err != nil {
			return fmt.Errorf("error al leer el bloque de directorio: %v", err)
		}

		for j := 0; j < len(dirBlock.B_content); j++ {
			entry := &dirBlock.B_content[j]
			if string(entry.B_name[:]) == oldName {
				// Depuración: Verificar el nombre antes de actualizar
				fmt.Printf("Depuración: Nombre encontrado en el bloque: '%s'\n", string(entry.B_name[:]))

				// Limpiar el espacio del nombre anterior
				for k := range entry.B_name {
					entry.B_name[k] = 0
				}

				// Actualizar el nombre
				copy(entry.B_name[:], newName)

				// Depuración: Verificar el nombre después de actualizar
				fmt.Printf("Depuración: Nombre actualizado en el bloque: '%s'\n", string(entry.B_name[:]))

				// Escribir el bloque actualizado de vuelta al disco
				err := utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+dirInode.Iblock[i]*ext3fs.SuperBlock.S_block_s), dirBlock)
				if err != nil {
					return fmt.Errorf("error al escribir el bloque de directorio actualizado: %v", err)
				}

				return nil
			}
		}
	}
	if dirInode.Iblock[12] != -1 {

		indirectBlock := make([]int32, ext3fs.SuperBlock.S_block_s/4)
		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+dirInode.Iblock[12]*ext3fs.SuperBlock.S_block_s), &indirectBlock)
		if err != nil {
			return fmt.Errorf("error al leer el bloque indirecto: %v", err)
		}

		for _, blockNum := range indirectBlock {
			if blockNum == -1 {
				continue
			}

			dirBlock := &bloques.BloqueCarpeta{}
			err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+blockNum*ext3fs.SuperBlock.S_block_s), dirBlock)
			if err != nil {
				return fmt.Errorf("error al leer el bloque de directorio: %v", err)
			}

			for j := 0; j < len(dirBlock.B_content); j++ {
				entry := &dirBlock.B_content[j]
				if string(entry.B_name[:]) == oldName {
					// Actualizar el nombre
					copy(entry.B_name[:], newName)

					// Escribir el bloque actualizado de vuelta al disco
					err := utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+blockNum*ext3fs.SuperBlock.S_block_s), dirBlock)
					if err != nil {
						return fmt.Errorf("error al escribir el bloque de directorio actualizado: %v", err)
					}

					return nil
				}
			}
		}
	}

	return fmt.Errorf("archivo '%s' no encontrado en el directorio", oldName)

}
