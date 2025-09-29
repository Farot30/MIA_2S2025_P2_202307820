package commandsP2

import (
	BloquesP2 "MIA_2S2025_P2_202307820/Bloques"
	EstructuraExt2P2 "MIA_2S2025_P2_202307820/EstructuraExt2"
	EstructuraExt3P2 "MIA_2S2025_P2_202307820/EstructuraExt3"
	sessionP2 "MIA_2S2025_P2_202307820/Session"
	EstructuraarchivocarpetaP2 "MIA_2S2025_P2_202307820/estructuraArchivoCarpeta"
	utilitiesP2 "MIA_2S2025_P2_202307820/utilities"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func COPY(args []string) {

	if !sessionP2.IsSessionActive() {
		fmt.Printf("ERROR: No active session. Please log in first.")
		return
	}

	params := utilitiesP2.ParseParameters(args)
	path := strings.Trim(params["path"], "\"")
	dest := strings.Trim(params["destino"], "\"")

	if path == "" || dest == "" {
		fmt.Println("ERROR: Faltan parámetros obligatorios para el comando copy: 'path' o 'destino'.")
		return
	}

	if !strings.HasPrefix(path, "/") || !strings.HasPrefix(dest, "/") {
		fmt.Println("ERROR: los parametros 'path' y 'destino' deben comenzar con '/'.")
		return
	}

	sessionInfo := sessionP2.GetCurrentSession()
	if sessionInfo == nil {
		fmt.Println("ERROR: no se pudo obtener información de la sesión actual\n.")
		return
	}

	MountedPartition, exist := GetMountedPartition(sessionInfo.PartitionID)
	if !exist {
		fmt.Println("ERROR: la partición no está montada.")
		return
	}

	// Abrir el archivo de la partición
	file, err := os.OpenFile(MountedPartition.Path, os.O_RDWR, 0666)
	if err != nil {
		fmt.Printf("ERROR: No se pudo abrir la partición: %v\n", err)
		return
	}
	defer file.Close()

	// Leer el superbloque
	superBlock := &EstructuraarchivocarpetaP2.SuperBloque{}
	err = utilitiesP2.ReadStruct(file, int64(MountedPartition.Start), superBlock)
	if err != nil {
		fmt.Printf("ERROR: No se pudo leer el superbloque: %v\n", err)
		return
	}

	// Verificar tipo de sistema de archivos
	if superBlock.S_filesystem_type != 2 && superBlock.S_filesystem_type != 3 {
		fmt.Printf("ERROR: Sistema de archivos no soportado: %d\n", superBlock.S_filesystem_type)
		return
	}

	// Para el caso de EXT2
	if superBlock.S_filesystem_type == 2 {
		// Leer el sistema de archivos EXT2
		ext2fs, err := EstructuraExt2P2.ReadEXT2FileSystem(MountedPartition.Path, MountedPartition.Start)
		if err != nil {
			fmt.Printf("ERROR: No se pudo leer el sistema de archivos EXT2: %v\n", err)
			return
		}

		// Encontrar el directorio padre de la ruta origen
		sourceParentPath := filepath.Dir(path)
		sourceParentInodeNum, err := findDirectoryInEXT2ForCopy(file, ext2fs, sourceParentPath)
		if err != nil {
			fmt.Printf("ERROR: No se pudo encontrar el directorio fuente '%s': %v\n", sourceParentPath, err)
			return
		}

		// Leer el inodo del directorio padre origen
		sourceParentInode := &EstructuraarchivocarpetaP2.Inodo{}
		err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+sourceParentInodeNum*ext2fs.SuperBlock.S_inode_s), sourceParentInode)
		if err != nil {
			fmt.Printf("ERROR: No se pudo leer el inodo del directorio fuente: %v\n", err)
			return
		}

		// Encontrar el archivo o directorio a copiar
		sourceName := filepath.Base(path)
		sourceInodeNum, err := findFileInDirectoryEXT2ForCopy(file, ext2fs, sourceParentInode, sourceName)
		if err != nil || sourceInodeNum == -1 {
			fmt.Printf("ERROR: No se pudo encontrar el archivo o directorio '%s': %v\n", sourceName, err)
			return
		}

		// Leer el inodo del archivo o directorio a copiar
		sourceInode := &EstructuraarchivocarpetaP2.Inodo{}
		err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+sourceInodeNum*ext2fs.SuperBlock.S_inode_s), sourceInode)
		if err != nil {
			fmt.Printf("ERROR: No se pudo leer el inodo de '%s': %v\n", sourceName, err)
			return
		}

		// Verificar permiso de lectura en el origen
		canRead, err := sessionP2.CheckPermissions(sourceInode, sessionP2.READ)
		if err != nil || !canRead {
			fmt.Printf("ERROR: No tiene permisos de lectura para '%s'\n", path)
			return
		}

		// Determinar si el destino es un directorio existente
		destInodeNum, err := findDirectoryInEXT2ForCopy(file, ext2fs, dest)
		if err == nil && destInodeNum != -1 {
			// El destino es un directorio existente, copiar dentro de él
			destParentInode := &EstructuraarchivocarpetaP2.Inodo{}
			_ = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+destInodeNum*ext2fs.SuperBlock.S_inode_s), destParentInode)
			fmt.Printf("[Depuración] COPY: Inodo destino %d, Itype=%d (0=dir,1=archivo)\n", destInodeNum, destParentInode.Itype)
			if destParentInode.Itype != 0 {
				fmt.Printf("ERROR: El destino '%s' no es un directorio. Abortando copia.\n", dest)
				return
			}
			canWrite, err := sessionP2.CheckPermissions(destParentInode, sessionP2.WRITE)
			if err != nil || !canWrite {
				fmt.Printf("ERROR: No tiene permisos de escritura en el directorio destino '%s'\n", dest)
				return
			}
			// El nombre destino será el nombre del archivo fuente
			destName := filepath.Base(path)
			// Verificar si ya existe un archivo con ese nombre en el directorio destino
			existingInodeNum, err := findFileInDirectoryEXT2ForCopy(file, ext2fs, destParentInode, destName)
			if err == nil && existingInodeNum != -1 {
				fmt.Printf("ERROR: Ya existe un archivo o directorio con el nombre '%s' en el destino\n", destName)
				return
			}
			fmt.Printf("[Depuración] COPY: Antes de copiar, inodo destino %d sigue siendo directorio.\n", destInodeNum)
			// Realizar la copia según el tipo de inodo (archivo o directorio)
			switch sourceInode.Itype {
			case 1: // Es un archivo
				err = copyFileInEXT2(file, ext2fs, sourceInode, sourceName, destParentInode, destName, sessionInfo, destInodeNum)
				if err != nil {
					fmt.Printf("ERROR: No se pudo copiar el archivo '%s': %v\n", sourceName, err)
					return
				}
			case 0: // Es un directorio
				err = copyDirectoryInEXT2(file, ext2fs, sourceInode, sourceName, destParentInode, destName, sessionInfo)
				if err != nil {
					fmt.Printf("ERROR: No se pudo copiar el directorio '%s': %v\n", sourceName, err)
					return
				}
			default:
				fmt.Printf("ERROR: Tipo de inodo desconocido para '%s'\n", sourceName)
				return
			}
			// Leer de nuevo el inodo destino tras la copia
			if err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+destInodeNum*ext2fs.SuperBlock.S_inode_s), destParentInode); err != nil {
				fmt.Printf("Advertencia: No se pudo releer el inodo destino: %v\n", err)
			}
			fmt.Printf("[Depuración] COPY: Después de copiar, inodo destino %d Itype=%d (0=dir,1=archivo)\n", destInodeNum, destParentInode.Itype)
			// Actualizar el superbloque
			err = utilitiesP2.WriteStruct(file, int64(MountedPartition.Start), superBlock)
			if err != nil {
				fmt.Printf("ERROR: No se pudo actualizar el superbloque: %v\n", err)
				return
			}
			fmt.Printf("SUCCESS: '%s' copiado a '%s'\n", path, dest)
			return
		}

		// Si no es un directorio existente, proceder como antes (destino es archivo nuevo o directorio inexistente)
		destDirPath := filepath.Dir(dest)
		destName := filepath.Base(dest)
		destParentInodeNum, err := findDirectoryInEXT2ForCopy(file, ext2fs, destDirPath)
		if err != nil {
			fmt.Printf("ERROR: No se pudo encontrar el directorio destino '%s': %v\n", destDirPath, err)
			return
		}
		destParentInode := &EstructuraarchivocarpetaP2.Inodo{}
		err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+destParentInodeNum*ext2fs.SuperBlock.S_inode_s), destParentInode)
		if err != nil {
			fmt.Printf("ERROR: No se pudo leer el inodo del directorio destino: %v\n", err)
			return
		}
		canWrite, err := sessionP2.CheckPermissions(destParentInode, sessionP2.WRITE)
		if err != nil || !canWrite {
			fmt.Printf("ERROR: No tiene permisos de escritura en el directorio destino '%s'\n", destDirPath)
			return
		}
		existingInodeNum, err := findFileInDirectoryEXT2ForCopy(file, ext2fs, destParentInode, destName)
		if err == nil && existingInodeNum != -1 {
			fmt.Printf("ERROR: Ya existe un archivo o directorio con el nombre '%s' en el destino\n", destName)
			return
		}

		// Realizar la copia según el tipo de inodo (archivo o directorio)
		switch sourceInode.Itype {
		case 1: // Es un archivo
			err = copyFileInEXT2(file, ext2fs, sourceInode, sourceName, destParentInode, destName, sessionInfo, destInodeNum)
			if err != nil {
				fmt.Printf("ERROR: No se pudo copiar el archivo '%s': %v\n", sourceName, err)
				return
			}
		case 0: // Es un directorio
			err = copyDirectoryInEXT2(file, ext2fs, sourceInode, sourceName, destParentInode, destName, sessionInfo)
			if err != nil {
				fmt.Printf("ERROR: No se pudo copiar el directorio '%s': %v\n", sourceName, err)
				return
			}
		default:
			fmt.Printf("ERROR: Tipo de inodo desconocido para '%s'\n", sourceName)
			return
		}

		// Actualizar el superbloque
		err = utilitiesP2.WriteStruct(file, int64(MountedPartition.Start), superBlock)
		if err != nil {
			fmt.Printf("ERROR: No se pudo actualizar el superbloque: %v\n", err)
			return
		}

		fmt.Printf("SUCCESS: '%s' copiado a '%s'\n", path, dest)
	} else if superBlock.S_filesystem_type == 3 {

		ext3fs, err := EstructuraExt3P2.ReadEXT3FileSystem(MountedPartition.Path, MountedPartition.Start)
		if err != nil {
			fmt.Printf("ERROR: No se pudo leer el sistema de archivos EXT3: %v\n", err)
			return
		}

		sourceParentPath := filepath.Dir(path)
		SourceParentInodeNum, err := findDirectoryInEXT3(file, ext3fs, sourceParentPath)
		if err != nil {
			fmt.Printf("ERROR: No se pudo encontrar el directorio fuente '%s': %v\n", sourceParentPath, err)
			return
		}

		SuurceParentInode := &EstructuraarchivocarpetaP2.Inodo{}
		err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+SourceParentInodeNum*ext3fs.SuperBlock.S_inode_s), SuurceParentInode)
		if err != nil {
			fmt.Printf("ERROR: No se pudo leer el inodo del directorio fuente: %v\n", err)
			return
		}

		sourceName := filepath.Base(path)
		SourceInodeNum, err := findFileInDirectoryEXT3(file, ext3fs, SuurceParentInode, sourceName)
		if err != nil {
			fmt.Printf("ERROR: No se pudo encontrar el archivo fuente '%s': %v\n", sourceName, err)
			return
		}

		sourceInode := &EstructuraarchivocarpetaP2.Inodo{}
		err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+SourceInodeNum*ext3fs.SuperBlock.S_inode_s), sourceInode)
		if err != nil {
			fmt.Printf("ERROR: No se pudo leer el inodo del archivo fuente: %v\n", err)
			return
		}

		canRead, err := sessionP2.CheckPermissions(sourceInode, sessionP2.READ)
		if err != nil || !canRead {
			fmt.Printf("ERROR: No tiene permisos de lectura para '%s'\n", path)
			return
		}

		destInodeNum, err := findDirectoryInEXT3(file, ext3fs, dest)
		if err == nil && destInodeNum != -1 {
			destParentInode := &EstructuraarchivocarpetaP2.Inodo{}
			_ = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+destInodeNum*ext3fs.SuperBlock.S_inode_s), destParentInode)
			fmt.Printf("[Depuración] COPY: Inodo destino %d, Itype=%d (0=dir,1=archivo)\n", destInodeNum, destParentInode.Itype)
			if destParentInode.Itype != 0 {
				fmt.Printf("ERROR: El destino '%s' no es un directorio. Abortando copia.\n", dest)
				return
			}
			canWrite, err := sessionP2.CheckPermissions(destParentInode, sessionP2.WRITE)
			if err != nil || !canWrite {
				fmt.Printf("ERROR: No tiene permisos de escritura en el directorio destino '%s'\n", dest)
				return
			}

			destName := filepath.Base(path)
			existingInodeNum, err := findFileInDirectoryEXT3(file, ext3fs, destParentInode, destName)
			if err == nil && existingInodeNum != -1 {
				fmt.Printf("ERROR: Ya existe un archivo o directorio con el nombre '%s' en el destino\n", destName)
				return
			}
			fmt.Printf("[Depuración] COPY: Antes de copiar, inodo destino %d sigue siendo directorio.\n", destInodeNum)

			switch sourceInode.Itype {
			case 1:
				err = copyFileInEXT3(file, ext3fs, sourceInode, sourceName, destParentInode, destName, sessionInfo, destInodeNum)
				if err != nil {
					fmt.Printf("ERROR: No se pudo copiar el archivo '%s': %v\n", sourceName, err)
					return
				}
			case 0:
				err = copyDirectoryInEXT3(file, ext3fs, sourceInode, sourceName, destParentInode, destName, sessionInfo)
				if err != nil {
					fmt.Printf("ERROR: No se pudo copiar el directorio '%s': %v\n", sourceName, err)
					return
				}
			default:
				fmt.Printf("ERROR: Tipo de inodo desconocido para '%s'\n", sourceName)
				return
			}

			if err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+destInodeNum*ext3fs.SuperBlock.S_inode_s), destParentInode); err != nil {
				fmt.Printf("Advertencia: No se pudo releer el inodo destino: %v\n", err)
			}
			fmt.Printf("[Depuración] COPY: Después de copiar, inodo destino %d Itype=%d (0=dir,1=archivo)\n", destInodeNum, destParentInode.Itype)

			err = utilitiesP2.WriteStruct(file, int64(MountedPartition.Start), superBlock)
			if err != nil {
				fmt.Printf("ERROR: No se pudo actualizar el superbloque: %v\n", err)
				return
			}
			fmt.Printf("SUCCESS: '%s' copiado a '%s'\n", path, dest)
			return
		}

		destDirPath := filepath.Dir(dest)
		destName := filepath.Base(dest)
		destParentInodeNum, err := findDirectoryInEXT3(file, ext3fs, destDirPath)
		if err != nil {
			fmt.Printf("ERROR: No se pudo encontrar el directorio destino '%s': %v\n", destDirPath, err)
			return
		}
		destParentInode := &EstructuraarchivocarpetaP2.Inodo{}
		err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+destParentInodeNum*ext3fs.SuperBlock.S_inode_s), destParentInode)
		if err != nil {
			fmt.Printf("ERROR: No se pudo leer el inodo del directorio destino: %v\n", err)
			return
		}
		canWrite, err := sessionP2.CheckPermissions(destParentInode, sessionP2.WRITE)
		if err != nil || !canWrite {
			fmt.Printf("ERROR: No tiene permisos de escritura en el directorio destino '%s'\n", destDirPath)
			return
		}
		existingInodeNum, err := findFileInDirectoryEXT3(file, ext3fs, destParentInode, destName)
		if err == nil && existingInodeNum != -1 {
			fmt.Printf("ERROR: Ya existe un archivo o directorio con el nombre '%s' en el destino\n", destName)
			return
		}

		switch sourceInode.Itype {
		case 1:
			err = copyFileInEXT3(file, ext3fs, sourceInode, sourceName, destParentInode, destName, sessionInfo, destInodeNum)
			if err != nil {
				fmt.Printf("ERROR: No se pudo copiar el archivo '%s': %v\n", sourceName, err)
				return
			}
		case 0:
			err = copyDirectoryInEXT3(file, ext3fs, sourceInode, sourceName, destParentInode, destName, sessionInfo)
			if err != nil {
				fmt.Printf("ERROR: No se pudo copiar el directorio '%s': %v\n", sourceName, err)
				return
			}
		default:
			fmt.Printf("ERROR: Tipo de inodo desconocido para '%s'\n", sourceName)
			return
		}

		err = utilitiesP2.WriteStruct(file, int64(MountedPartition.Start), superBlock)
		if err != nil {
			fmt.Printf("ERROR: No se pudo actualizar el superbloque: %v\n", err)
			return
		}

		fmt.Printf("SUCCESS: '%s' copiado a '%s'\n", path, dest)
	}
}

func copyFileInEXT2(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, sourceInode *EstructuraarchivocarpetaP2.Inodo, sourceName string, destInode *EstructuraarchivocarpetaP2.Inodo, destName string, _ *sessionP2.UserSession, destInodeNum int32) error {
	// Verificar que el inodo fuente es un archivo
	if sourceInode.Itype != 1 {
		fmt.Printf("Error: El inodo fuente no es un archivo (Itype=%d)\n", sourceInode.Itype)
		return nil
	}

	// DEPURACIÓN: Verificar inodo fuente
	fmt.Printf("[Depuración-CopyFile] Copiando archivo '%s'\n", sourceName)
	fmt.Printf("[Depuración-CopyFile] Tipo inodo: %d (debe ser 1=archivo)\n", sourceInode.Itype)
	fmt.Printf("[Depuración-CopyFile] Tamaño archivo: %d bytes\n", sourceInode.GetIs())

	// Verificar permisos de lectura en el archivo fuente
	canRead, err := sessionP2.CheckPermissions(sourceInode, sessionP2.READ)
	if err != nil || !canRead {
		fmt.Printf("Error: no tiene permisos de lectura para el archivo '%s': %v\n", sourceName, err)
		return err
	}

	// Verificar permisos de escritura en el directorio destino
	canWrite, err := sessionP2.CheckPermissions(destInode, sessionP2.WRITE)
	if err != nil || !canWrite {
		fmt.Printf("Error: no tiene permisos de escritura en el directorio destino para '%s': %v\n", destName, err)
		return err
	}

	// Crear un nuevo inodo para el archivo en el destino
	newInodeNum, err := findFreeInodeEXT2(file, ext2fs)
	if err != nil {
		fmt.Printf("Error: no se pudo encontrar un inodo libre para el archivo destino: %v\n", err)
		return err
	}

	// Verificar que no se esté usando el inodo #0 (raíz) para un archivo nuevo
	if newInodeNum == 0 {
		fmt.Printf("Error crítico: Se intentó asignar el inodo raíz (#0) para un archivo. Buscando otro inodo.\n")
		// Intentar encontrar otro inodo libre
		newInodeNum, err = findFreeInodeEXT2(file, ext2fs)
		if err != nil || newInodeNum <= 0 {
			fmt.Printf("Error: no se pudo encontrar un inodo libre alternativo: %v\n", err)
			return fmt.Errorf("error de seguridad: se intentó sobrescribir el inodo raíz")
		}
	}

	// Copiar propiedades del inodo fuente al nuevo inodo
	newInode := &EstructuraarchivocarpetaP2.Inodo{}
	*newInode = *sourceInode      // Copiar todos los atributos
	newInode.Iblock = [16]int32{} // Limpiar los bloques
	for i := 0; i < 16; i++ {
		newInode.Iblock[i] = -1 // Marcar todos los bloques como no usados
	}
	newInode.Is = sourceInode.Is // Mantener el tamaño original

	// SOLUCIÓN: Usar las funciones corregidas para leer y escribir el contenido del archivo
	fmt.Printf("[SOLUCIÓN] Utilizando método corregido para copiar el contenido del archivo\n")

	// 1. Leer correctamente todo el contenido del archivo fuente
	sourcePath := "/recovery/" + sourceName
	fileContent, err := GetActualFileContent(file, ext2fs, sourceInode, sourceName, sourcePath)
	if err != nil {
		fmt.Printf("[ERROR-SOLUCIÓN] No se pudo leer el contenido del archivo: %v\n", err)
		return err
	}

	fmt.Printf("[SOLUCIÓN] Contenido leído correctamente: '%s' (%d bytes)\n",
		string(fileContent), len(fileContent))

	// 2. Escribir correctamente el contenido al archivo destino
	err = WriteFileContentCorrect(file, ext2fs, newInode, fileContent)
	if err != nil {
		fmt.Printf("[ERROR-SOLUCIÓN] No se pudo escribir el contenido al archivo: %v\n", err)
		return err
	}

	fmt.Printf("[SOLUCIÓN] Contenido escrito correctamente al nuevo inodo\n")

	// Para compatibilidad con el resto del código, añadimos una pequeña verificación
	if len(fileContent) > 0 && newInode.Iblock[0] != -1 {
		fmt.Printf("[VERIFICACIÓN] Primer bloque del nuevo archivo es #%d\n", newInode.Iblock[0])
		verifyBlock := &BloquesP2.BloqueArchivo{}
		blockOffset := int64(ext2fs.SuperBlock.S_block_start + newInode.Iblock[0]*ext2fs.SuperBlock.S_block_s)
		err := utilitiesP2.ReadStruct(file, blockOffset, verifyBlock)
		if err == nil {
			fmt.Printf("[VERIFICACIÓN] Contenido del primer bloque: '%s'\n",
				strings.ReplaceAll(string(verifyBlock.B_content[:30]), "\x00", "\\0"))
		}
	}

	// Para debugging, mostrar información de los bloques asignados
	fmt.Printf("[Depuración-CopyFile] Bloques asignados al archivo destino:\n")
	for i := 0; i < 12 && sourceInode.Iblock[i] != -1; i++ {
		sourceBlockNum := sourceInode.Iblock[i]

		// Leer el bloque fuente exactamente como está
		sourceBlock := &BloquesP2.BloqueArchivo{}
		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+sourceBlockNum*ext2fs.SuperBlock.S_block_s), sourceBlock)
		if err != nil {
			fmt.Printf("[ERROR-COPIA] No se pudo leer el bloque fuente #%d: %v\n", sourceBlockNum, err)
			return err
		}

		// Encontrar un bloque libre para el destino
		destBlockNum, err := EstructuraExt2P2.FindFreeBlockEXT2(file, ext2fs)
		if err != nil || destBlockNum == -1 {
			fmt.Printf("[ERROR-COPIA] No se pudo encontrar un bloque libre: %v\n", err)
			return err
		}

		// Escribir el bloque exactamente como está, sin interpretarlo
		err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+destBlockNum*ext2fs.SuperBlock.S_block_s), sourceBlock)
		if err != nil {
			fmt.Printf("[ERROR-COPIA] No se pudo escribir el bloque destino #%d: %v\n", destBlockNum, err)
			return err
		}

		// Actualizar el inodo destino
		newInode.Iblock[i] = destBlockNum

		fmt.Printf("[COPIA-DIRECTA] Bloque #%d copiado a bloque #%d\n", sourceBlockNum, destBlockNum)
	}
	// --- Bloque indirecto simple (12) ---
	if sourceInode.Iblock[12] != -1 {
		sourcePtrBlock := &BloquesP2.BloqueApuntador{}
		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+sourceInode.Iblock[12]*ext2fs.SuperBlock.S_block_s), sourcePtrBlock)
		if err != nil {
			fmt.Printf("Error: no se pudo leer el bloque apuntador fuente (simple): %v\n", err)
			return err
		}
		destPtrBlockNum, err := EstructuraExt2P2.FindFreeBlockEXT2(file, ext2fs)
		if err != nil || destPtrBlockNum == -1 {
			fmt.Printf("Error: no se pudo encontrar un bloque libre para el bloque apuntador destino (simple)\n")
			return err
		}
		newPtrBlock := &BloquesP2.BloqueApuntador{}
		for i, ptr := range sourcePtrBlock.B_pointers {
			if ptr == -1 {
				newPtrBlock.B_pointers[i] = -1
				continue
			}
			sourceDataBlock := &BloquesP2.BloqueArchivo{}
			err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+ptr*ext2fs.SuperBlock.S_block_s), sourceDataBlock)
			if err != nil {
				fmt.Printf("Error: no se pudo leer el bloque de datos fuente (indirecto simple): %v\n", err)
				return err
			}
			destDataBlockNum, err := EstructuraExt2P2.FindFreeBlockEXT2(file, ext2fs)
			if err != nil || destDataBlockNum == -1 {
				fmt.Printf("Error: no se pudo encontrar un bloque libre para el bloque de datos destino (indirecto simple)\n")
				return err
			}
			err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+destDataBlockNum*ext2fs.SuperBlock.S_block_s), sourceDataBlock)
			if err != nil {
				fmt.Printf("Error: no se pudo escribir el bloque de datos destino (indirecto simple): %v\n", err)
				return err
			}
			newPtrBlock.B_pointers[i] = destDataBlockNum
		}
		err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+destPtrBlockNum*ext2fs.SuperBlock.S_block_s), newPtrBlock)
		if err != nil {
			fmt.Printf("Error: no se pudo escribir el bloque apuntador destino (simple): %v\n", err)
			return err
		}
		newInode.Iblock[12] = destPtrBlockNum
	}

	// --- Bloque indirecto doble (13) ---
	if sourceInode.Iblock[13] != -1 {
		sourceDoublePtrBlock := &BloquesP2.BloqueApuntador{}
		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+sourceInode.Iblock[13]*ext2fs.SuperBlock.S_block_s), sourceDoublePtrBlock)
		if err != nil {
			fmt.Printf("Error: no se pudo leer el bloque apuntador fuente (doble): %v\n", err)
			return err
		}
		destDoublePtrBlockNum, err := EstructuraExt2P2.FindFreeBlockEXT2(file, ext2fs)
		if err != nil || destDoublePtrBlockNum == -1 {
			fmt.Printf("Error: no se pudo encontrar un bloque libre para el bloque apuntador destino (doble)\n")
			return err
		}
		newDoublePtrBlock := &BloquesP2.BloqueApuntador{}
		for i, ptr := range sourceDoublePtrBlock.B_pointers {
			if ptr == -1 {
				newDoublePtrBlock.B_pointers[i] = -1
				continue
			}
			sourceSimplePtrBlock := &BloquesP2.BloqueApuntador{}
			err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+ptr*ext2fs.SuperBlock.S_block_s), sourceSimplePtrBlock)
			if err != nil {
				fmt.Printf("Error: no se pudo leer el bloque apuntador simple (doble indirecto): %v\n", err)
				return err
			}
			destSimplePtrBlockNum, err := EstructuraExt2P2.FindFreeBlockEXT2(file, ext2fs)
			if err != nil || destSimplePtrBlockNum == -1 {
				fmt.Printf("Error: no se pudo encontrar un bloque libre para el bloque apuntador simple (doble indirecto)\n")
				return err
			}
			newSimplePtrBlock := &BloquesP2.BloqueApuntador{}
			for j, ptr2 := range sourceSimplePtrBlock.B_pointers {
				if ptr2 == -1 {
					newSimplePtrBlock.B_pointers[j] = -1
					continue
				}
				sourceDataBlock := &BloquesP2.BloqueArchivo{}
				err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+ptr2*ext2fs.SuperBlock.S_block_s), sourceDataBlock)
				if err != nil {
					fmt.Printf("Error: no se pudo leer el bloque de datos fuente (doble indirecto): %v\n", err)
					return err
				}
				destDataBlockNum, err := EstructuraExt2P2.FindFreeBlockEXT2(file, ext2fs)
				if err != nil || destDataBlockNum == -1 {
					fmt.Printf("Error: no se pudo encontrar un bloque libre para el bloque de datos destino (doble indirecto)\n")
					return err
				}
				err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+destDataBlockNum*ext2fs.SuperBlock.S_block_s), sourceDataBlock)
				if err != nil {
					fmt.Printf("Error: no se pudo escribir el bloque de datos destino (doble indirecto): %v\n", err)
					return err
				}
				newSimplePtrBlock.B_pointers[j] = destDataBlockNum
			}
			err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+destSimplePtrBlockNum*ext2fs.SuperBlock.S_block_s), newSimplePtrBlock)
			if err != nil {
				fmt.Printf("Error: no se pudo escribir el bloque apuntador destino (doble): %v\n", err)
				return err
			}
			newDoublePtrBlock.B_pointers[i] = destSimplePtrBlockNum
		}
		err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+destDoublePtrBlockNum*ext2fs.SuperBlock.S_block_s), newDoublePtrBlock)
		if err != nil {
			fmt.Printf("Error: no se pudo escribir el bloque apuntador destino (doble): %v\n", err)
			return err
		}
		newInode.Iblock[13] = destDoublePtrBlockNum
	}

	// --- Bloque indirecto triple (14) ---
	if sourceInode.Iblock[14] != -1 {
		sourceTriplePtrBlock := &BloquesP2.BloqueApuntador{}
		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+sourceInode.Iblock[14]*ext2fs.SuperBlock.S_block_s), sourceTriplePtrBlock)
		if err != nil {
			fmt.Printf("Error: no se pudo leer el bloque apuntador fuente (triple): %v\n", err)
			return err
		}
		destTriplePtrBlockNum, err := EstructuraExt2P2.FindFreeBlockEXT2(file, ext2fs)
		if err != nil || destTriplePtrBlockNum == -1 {
			fmt.Printf("Error: no se pudo encontrar un bloque libre para el bloque apuntador destino (triple)\n")
			return err
		}
		newTriplePtrBlock := &BloquesP2.BloqueApuntador{}
		for i, ptr := range sourceTriplePtrBlock.B_pointers {
			if ptr == -1 {
				newTriplePtrBlock.B_pointers[i] = -1
				continue
			}
			sourceDoublePtrBlock := &BloquesP2.BloqueApuntador{}
			err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+ptr*ext2fs.SuperBlock.S_block_s), sourceDoublePtrBlock)
			if err != nil {
				fmt.Printf("Error: no se pudo leer el bloque apuntador doble (triple indirecto): %v\n", err)
				return err
			}
			destDoublePtrBlockNum, err := EstructuraExt2P2.FindFreeBlockEXT2(file, ext2fs)
			if err != nil || destDoublePtrBlockNum == -1 {
				fmt.Printf("Error: no se pudo encontrar un bloque libre para el bloque apuntador doble (triple indirecto)\n")
				return err
			}
			newDoublePtrBlock := &BloquesP2.BloqueApuntador{}
			for j, ptr2 := range sourceDoublePtrBlock.B_pointers {
				if ptr2 == -1 {
					newDoublePtrBlock.B_pointers[j] = -1
					continue
				}
				sourceSimplePtrBlock := &BloquesP2.BloqueApuntador{}
				err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+ptr2*ext2fs.SuperBlock.S_block_s), sourceSimplePtrBlock)
				if err != nil {
					fmt.Printf("Error: no se pudo leer el bloque apuntador simple (triple indirecto): %v\n", err)
					return err
				}
				destSimplePtrBlockNum, err := EstructuraExt2P2.FindFreeBlockEXT2(file, ext2fs)
				if err != nil || destSimplePtrBlockNum == -1 {
					fmt.Printf("Error: no se pudo encontrar un bloque libre para el bloque apuntador simple (triple indirecto)\n")
					return err
				}
				newSimplePtrBlock := &BloquesP2.BloqueApuntador{}
				for k, ptr3 := range sourceSimplePtrBlock.B_pointers {
					if ptr3 == -1 {
						newSimplePtrBlock.B_pointers[k] = -1
						continue
					}
					sourceDataBlock := &BloquesP2.BloqueArchivo{}
					err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+ptr3*ext2fs.SuperBlock.S_block_s), sourceDataBlock)
					if err != nil {
						fmt.Printf("Error: no se pudo leer el bloque de datos fuente (triple indirecto): %v\n", err)
						return err
					}
					destDataBlockNum, err := EstructuraExt2P2.FindFreeBlockEXT2(file, ext2fs)
					if err != nil || destDataBlockNum == -1 {
						fmt.Printf("Error: no se pudo encontrar un bloque libre para el bloque de datos destino (triple indirecto)\n")
						return err
					}
					err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+destDataBlockNum*ext2fs.SuperBlock.S_block_s), sourceDataBlock)
					if err != nil {
						fmt.Printf("Error: no se pudo escribir el bloque de datos destino (triple indirecto): %v\n", err)
						return err
					}
					newSimplePtrBlock.B_pointers[k] = destDataBlockNum
				}
				err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+destSimplePtrBlockNum*ext2fs.SuperBlock.S_block_s), newSimplePtrBlock)
				if err != nil {
					fmt.Printf("Error: no se pudo escribir el bloque apuntador destino (triple): %v\n", err)
					return err
				}
				newDoublePtrBlock.B_pointers[j] = destSimplePtrBlockNum
			}
			err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+destDoublePtrBlockNum*ext2fs.SuperBlock.S_block_s), newDoublePtrBlock)
			if err != nil {
				fmt.Printf("Error: no se pudo escribir el bloque apuntador doble destino (triple indirecto): %v\n", err)
				return err
			}
			newTriplePtrBlock.B_pointers[i] = destDoublePtrBlockNum
		}
		err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+destTriplePtrBlockNum*ext2fs.SuperBlock.S_block_s), newTriplePtrBlock)
		if err != nil {
			fmt.Printf("Error: no se pudo escribir el bloque apuntador destino (triple): %v\n", err)
			return err
		}
		newInode.Iblock[14] = destTriplePtrBlockNum
	}

	// Depuración antes de escribir el nuevo inodo
	fmt.Printf("[Depuración] copyFileInEXT2: Antes de escribir inodo destino #%d, Itype=%d (0=dir,1=archivo)\n", newInodeNum, newInode.Itype)
	err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_inode_start+newInodeNum*ext2fs.SuperBlock.S_inode_s), newInode)
	if err != nil {
		fmt.Printf("Error: no se pudo escribir el nuevo inodo en el disco: %v\n", err)
		return err
	}
	// Depuración después de escribir el nuevo inodo
	tmpInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+newInodeNum*ext2fs.SuperBlock.S_inode_s), tmpInode)
	if err == nil {
		fmt.Printf("[Depuración] copyFileInEXT2: Después de escribir inodo #%d, Itype=%d (0=dir,1=archivo)\n", newInodeNum, tmpInode.Itype)
	}

	currentDestInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+destInodeNum*ext2fs.SuperBlock.S_inode_s), currentDestInode)
	if err == nil {
		// Usar el inodo actualizado para agregar la entrada
		destInode = currentDestInode
	}

	fmt.Printf("[Depuración] copyFileInEXT2: Agregando archivo '%s' (inodo #%d) al directorio #%d\n", destName, newInodeNum, destInodeNum)

	// Agregar la entrada al directorio destino
	if !addEntryToDirectoryEXT2(file, ext2fs, destInode, destInodeNum, destName, newInodeNum) {
		fmt.Printf("Error: No se pudo agregar la entrada '%s' al directorio destino\n", destName)
		return fmt.Errorf("no se pudo agregar la entrada '%s' al directorio destino", destName)
	}

	fmt.Printf("[Depuración] copyFileInEXT2: Archivo '%s' agregado exitosamente al directorio #%d\n", destName, destInodeNum)
	return nil
}

func copyDirectoryInEXT2(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, sourceInode *EstructuraarchivocarpetaP2.Inodo, sourceName string, destInode *EstructuraarchivocarpetaP2.Inodo, destName string, sessionInfo *sessionP2.UserSession) error {
	// Verificar que el inodo fuente es un directorio
	if sourceInode.Itype != 0 {
		fmt.Printf("Error: El inodo fuente no es un directorio (Itype=%d)\n", sourceInode.Itype)
		return nil
	}

	// Verificar permisos de lectura en el directorio fuente
	canRead, err := sessionP2.CheckPermissions(sourceInode, sessionP2.READ)
	if err != nil || !canRead {
		fmt.Printf("Error: no tiene permisos de lectura para el directorio '%s': %v\n", sourceName, err)
		return err
	}

	// Verificar permisos de escritura en el directorio destino
	canWrite, err := sessionP2.CheckPermissions(destInode, sessionP2.WRITE)
	if err != nil || !canWrite {
		fmt.Printf("Error: no tiene permisos de escritura en el directorio destino para '%s': %v\n", destName, err)
		return err
	}

	// Crear un nuevo inodo para el directorio en el destino
	newInodeNum, err := findFreeInodeEXT2(file, ext2fs)
	if err != nil {
		fmt.Printf("Error: no se pudo encontrar un inodo libre para el directorio destino: %v\n", err)
		return err
	}

	// Copiar propiedades del inodo fuente al nuevo inodo
	newInode := &EstructuraarchivocarpetaP2.Inodo{}
	*newInode = *sourceInode      // Copiar todos los atributos
	newInode.Iblock = [16]int32{} // Limpiar los bloques
	for i := 0; i < 16; i++ {
		newInode.Iblock[i] = -1 // Marcar todos los bloques como no usados
	}
	newInode.Is = 0 // Reiniciar el tamaño

	// Crear el bloque de carpeta para el nuevo directorio
	newDirBlock := BloquesP2.NewBloqueCarpeta()

	// Encontrar un bloque libre para el directorio
	newBlockNum, err := EstructuraExt2P2.FindFreeBlockEXT2(file, ext2fs)
	if err != nil {
		return fmt.Errorf("error al buscar bloque libre: %v", err)
	}

	// Asignar el bloque al inodo
	newInode.Iblock[0] = newBlockNum

	// Determinamos el número de inodo del directorio padre (destino)
	// Es fundamental identificar correctamente este inodo para mantener la estructura del directorio

	// Inicializar el inodo padre
	var parentInodeNum int32 = 0 // Por defecto, usar raíz

	// Si tenemos el dirInodeNum en la función que llamó a esta, podemos usarlo directamente
	if destInode != nil {
		// Buscar en la tabla de inodos para encontrar el número de este inodo
		for i := int32(0); i < ext2fs.SuperBlock.S_inodes_count && parentInodeNum == 0; i++ {
			tempInode := &EstructuraarchivocarpetaP2.Inodo{}
			if err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+i*ext2fs.SuperBlock.S_inode_s), tempInode); err == nil {
				// Comparación simple pero efectiva de inodos
				if tempInode.Iblock[0] == destInode.Iblock[0] && tempInode.Itype == destInode.Itype {
					parentInodeNum = i
					fmt.Printf("[Depuración] copyDirectoryInEXT2: Directorio destino identificado como inodo #%d\n", parentInodeNum)
					break
				}
			}
		}
	}

	// Búsqueda adicional en la tabla de inodos para verificación
	for i := int32(0); i < ext2fs.SuperBlock.S_inodes_count; i++ {
		tempInode := &EstructuraarchivocarpetaP2.Inodo{}
		if err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+i*ext2fs.SuperBlock.S_inode_s), tempInode); err == nil {
			// Comparamos varios atributos para ver si es el mismo inodo
			if tempInode.Itype == destInode.Itype &&
				tempInode.Iuid == destInode.Iuid &&
				tempInode.Iblock[0] == destInode.Iblock[0] {
				parentInodeNum = i
				fmt.Printf("[Depuración] copyDirectoryInEXT2: Encontrado inodo padre #%d\n", parentInodeNum)
				break
			}
		}
	}

	fmt.Printf("[Depuración] copyDirectoryInEXT2: Usando inodo padre #%d para la entrada '..'\n", parentInodeNum)

	// Agregar entradas '.' y '..' al bloque de directorio
	newDirBlock.AddEntry(".", newInodeNum)
	newDirBlock.AddEntry("..", parentInodeNum) // Usar el inodo padre encontrado

	// Verificar las entradas del directorio antes de escribirlo
	fmt.Printf("[Depuración] copyDirectoryInEXT2: Verificando entradas del nuevo directorio antes de escribir:\n")
	for j := 0; j < 4; j++ {
		if !newDirBlock.IsEntryEmpty(j) {
			entryName := strings.TrimRight(string(newDirBlock.B_content[j].B_name[:]), "\x00")
			fmt.Printf("[Depuración]   - Entrada: '%s' -> inodo #%d\n", entryName, newDirBlock.B_content[j].B_inodo)
		}
	}

	// Escribir el bloque de directorio
	err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+newBlockNum*ext2fs.SuperBlock.S_block_s), newDirBlock)
	if err != nil {
		fmt.Printf("Error: no se pudo escribir el bloque del nuevo directorio: %v\n", err)
		return err
	}

	// Verificar que el bloque se escribió correctamente
	verifyBlock := &BloquesP2.BloqueCarpeta{}
	if err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+newBlockNum*ext2fs.SuperBlock.S_block_s), verifyBlock); err == nil {
		fmt.Printf("[Depuración] copyDirectoryInEXT2: Verificando bloque después de escribir:\n")
		for j := 0; j < 4; j++ {
			if !verifyBlock.IsEntryEmpty(j) {
				entryName := strings.TrimRight(string(verifyBlock.B_content[j].B_name[:]), "\x00")
				fmt.Printf("[Depuración]   - Entrada: '%s' -> inodo #%d\n", entryName, verifyBlock.B_content[j].B_inodo)
			}
		}
	}

	// Depuración antes de escribir el nuevo inodo de directorio
	fmt.Printf("[Depuración] copyDirectoryInEXT2: Antes de escribir inodo destino #%d, Itype=%d (0=dir,1=archivo)\n", newInodeNum, newInode.Itype)
	err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_inode_start+newInodeNum*ext2fs.SuperBlock.S_inode_s), newInode)
	if err != nil {
		fmt.Printf("Error: no se pudo escribir el nuevo inodo en el disco: %v\n", err)
		return err
	}
	// Depuración después de escribir el nuevo inodo
	tmpInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+newInodeNum*ext2fs.SuperBlock.S_inode_s), tmpInode)
	if err == nil {
		fmt.Printf("[Depuración] copyDirectoryInEXT2: Después de escribir inodo #%d, Itype=%d (0=dir,1=archivo)\n", newInodeNum, tmpInode.Itype)
	}

	// IMPORTANTE: Agregar la entrada del nuevo directorio al directorio padre destino
	// Para encontrar el número de inodo destino, buscamos en los inodos
	destInodeNum := int32(-1)

	// Este bucle es crítico para que los nuevos directorios sean enlazados correctamente
	for i := int32(0); i < ext2fs.SuperBlock.S_inodes_count; i++ {
		tempInode := &EstructuraarchivocarpetaP2.Inodo{}
		if err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+i*ext2fs.SuperBlock.S_inode_s), tempInode); err == nil {
			// Comparamos los bloques y otros atributos para ver si es el mismo inodo (destino)
			// La comparación debe ser muy precisa para identificar correctamente el directorio padre
			if tempInode.Iblock[0] == destInode.Iblock[0] &&
				tempInode.Itype == destInode.Itype &&
				tempInode.Iuid == destInode.Iuid {
				destInodeNum = i

				// Verificación adicional: comprobar si tiene las entradas "." y ".."
				dirBlock := &BloquesP2.BloqueCarpeta{}
				if err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+tempInode.Iblock[0]*ext2fs.SuperBlock.S_block_s), dirBlock); err == nil {
					fmt.Printf("[Depuración] copyDirectoryInEXT2: Verificando entradas del directorio #%d:\n", i)
					for j := 0; j < 4; j++ {
						if dirBlock.B_content[j].B_inodo != -1 {
							entryName := strings.TrimRight(string(dirBlock.B_content[j].B_name[:]), "\x00")
							fmt.Printf("[Depuración]   - Entrada: '%s' -> inodo #%d\n", entryName, dirBlock.B_content[j].B_inodo)
						}
					}
				}

				break
			}
		}
	}

	if destInodeNum == -1 {
		fmt.Printf("Error: no se pudo determinar el número de inodo del directorio destino\n")
		// Usar el inodo padre que ya determinamos anteriormente para mantener consistencia
		fmt.Printf("[Depuración] copyDirectoryInEXT2: Intentando encontrar directorio '%s' desde la raíz\n", destName)
		fmt.Printf("[Depuración] copyDirectoryInEXT2: Para mantener consistencia, usando inodo padre #%d\n", parentInodeNum)

		destInodeNum = parentInodeNum
	}

	// IMPORTANTE: Refrescar el inodo destino antes de usarlo para asegurar que tenemos la información más actualizada
	// Esto evita problemas de datos desactualizados o corruptos
	freshDestInode := &EstructuraarchivocarpetaP2.Inodo{}
	if err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+destInodeNum*ext2fs.SuperBlock.S_inode_s), freshDestInode); err != nil {
		fmt.Printf("Error: no se pudo leer el inodo destino #%d: %v\n", destInodeNum, err)
		return fmt.Errorf("error leyendo inodo destino: %v", err)
	}

	// Verificar que el inodo destino es un directorio
	if freshDestInode.Itype != 0 {
		fmt.Printf("Error: el inodo destino #%d no es un directorio (tipo=%d)\n", destInodeNum, freshDestInode.Itype)
		return fmt.Errorf("el destino no es un directorio")
	}

	fmt.Printf("[Depuración] copyDirectoryInEXT2: Usando inodo destino #%d para agregar entrada\n", destInodeNum)

	if !addEntryToDirectoryEXT2(file, ext2fs, freshDestInode, destInodeNum, destName, newInodeNum) {
		fmt.Printf("Error: no se pudo agregar la entrada '%s' al directorio destino\n", destName)
		return fmt.Errorf("no se pudo agregar la entrada al directorio destino")
	}
	fmt.Printf("[Depuración] copyDirectoryInEXT2: Directorio '%s' agregado al directorio destino como inodo #%d\n", destName, newInodeNum)

	// Recorrer el contenido de la carpeta fuente
	var copyErrors []string

	blocks := sourceInode.Iblock

	for i := 0; i < 12; i++ {
		if sourceInode.Iblock[i] == -1 {
			continue // Bloque no asignado
		}

		sourceBlock := &BloquesP2.BloqueCarpeta{}
		err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+sourceInode.Iblock[i]*ext2fs.SuperBlock.S_block_s), sourceBlock)
		if err != nil {
			copyErrors = append(copyErrors, fmt.Sprintf("No se pudo leer el bloque %d del directorio fuente: %v", i, err))
			continue
		}

		for _, entry := range sourceBlock.B_content {
			entryName := strings.TrimRight(string(entry.B_name[:]), "\x00")
			if entry.B_inodo == -1 || entryName == "." || entryName == ".." {
				continue // Ignorar entradas especiales o vacías
			}

			entryInode := &EstructuraarchivocarpetaP2.Inodo{}
			err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+entry.B_inodo*ext2fs.SuperBlock.S_inode_s), entryInode)
			if err != nil {
				copyErrors = append(copyErrors, fmt.Sprintf("No se pudo leer el inodo de '%s': %v", entryName, err))
				continue
			}

			// Verificar permisos de lectura para este archivo/directorio
			canReadEntry, err := sessionP2.CheckPermissions(entryInode, sessionP2.READ)
			if err != nil || !canReadEntry {
				copyErrors = append(copyErrors, fmt.Sprintf("No se pudo copiar '%s': falta permiso de lectura", entryName))
				continue
			}

			switch entryInode.Itype {
			case 1: // Archivo
				fmt.Printf("[Depuración] copyDirectoryInEXT2: Copiando archivo '%s' al directorio #%d\n", entryName, newInodeNum)
				// Usar el inodo nuevo como destino y su número como destInodeNum
				err = copyFileInEXT2(file, ext2fs, entryInode, entryName, newInode, entryName, sessionInfo, newInodeNum)
				if err != nil {
					copyErrors = append(copyErrors, fmt.Sprintf("Error al copiar el archivo '%s': %v", entryName, err))
				} else {
					fmt.Printf("[Depuración] copyDirectoryInEXT2: Archivo '%s' copiado exitosamente al nuevo directorio\n", entryName)
				}
			case 0: // Directorio
				fmt.Printf("[Depuración] copyDirectoryInEXT2: Copiando subdirectorio '%s' al directorio #%d\n", entryName, newInodeNum)
				err = copyDirectoryInEXT2(file, ext2fs, entryInode, entryName, newInode, entryName, sessionInfo) // Usar sessionInfo
				if err != nil {
					copyErrors = append(copyErrors, fmt.Sprintf("Error al copiar el directorio '%s': %v", entryName, err))
				} else {
					fmt.Printf("[Depuración] copyDirectoryInEXT2: Subdirectorio '%s' copiado exitosamente\n", entryName)
				}
			default:
				copyErrors = append(copyErrors, fmt.Sprintf("Tipo de inodo desconocido para '%s': %d", entryName, entryInode.Itype))
			}
		}
	}

	if blocks[12] != -1 {
		sourceBlock := &BloquesP2.BloqueApuntador{}
		err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+sourceInode.Iblock[12]*ext2fs.SuperBlock.S_block_s), sourceBlock)
		if err != nil {
			copyErrors = append(copyErrors, fmt.Sprintf("No se pudo leer el bloque apuntador simple del directorio fuente: %v", err))
		} else {
			for _, ptr := range sourceBlock.B_pointers {
				if ptr == -1 {
					continue
				}
				indirectBlock := &BloquesP2.BloqueCarpeta{}
				err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+ptr*ext2fs.SuperBlock.S_block_s), indirectBlock)
				if err != nil {
					copyErrors = append(copyErrors, fmt.Sprintf("No se pudo leer el bloque de directorio indirecto: %v", err))
					continue
				}
				for _, entry := range indirectBlock.B_content {
					entryName := strings.TrimRight(string(entry.B_name[:]), "\x00")
					if entry.B_inodo == -1 || entryName == "." || entryName == ".." {
						continue // Ignorar entradas especiales o vacías
					}

					entryInode := &EstructuraarchivocarpetaP2.Inodo{}
					err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+entry.B_inodo*ext2fs.SuperBlock.S_inode_s), entryInode)
					if err != nil {
						copyErrors = append(copyErrors, fmt.Sprintf("No se pudo leer el inodo de '%s': %v", entryName, err))
						continue
					}

					// Verificar permisos de lectura para este archivo/directorio
					canReadEntry, err := sessionP2.CheckPermissions(entryInode, sessionP2.READ)
					if err != nil || !canReadEntry {
						copyErrors = append(copyErrors, fmt.Sprintf("No se pudo copiar '%s': falta permiso de lectura", entryName))
						continue
					}

					switch entryInode.Itype {
					case 1: // Archivo
						fmt.Printf("[Depuración] copyDirectoryInEXT2: Copiando archivo '%s' (indirecto) al directorio #%d\n", entryName, newInodeNum)
						// Usar el inodo nuevo como destino y su número como destInodeNum
						err = copyFileInEXT2(file, ext2fs, entryInode, entryName, newInode, entryName, sessionInfo, newInodeNum)
						if err != nil {
							copyErrors = append(copyErrors, fmt.Sprintf("Error al copiar el archivo '%s': %v", entryName, err))
						} else {
							fmt.Printf("[Depuración] copyDirectoryInEXT2: Archivo '%s' (indirecto) copiado exitosamente al nuevo directorio\n", entryName)
						}
					case 0: // Directorio
						fmt.Printf("[Depuración] copyDirectoryInEXT2: Copiando subdirectorio '%s' (indirecto) al directorio #%d\n", entryName, newInodeNum)
						err = copyDirectoryInEXT2(file, ext2fs, entryInode, entryName, newInode, entryName, sessionInfo) // Usar sessionInfo
						if err != nil {
							copyErrors = append(copyErrors, fmt.Sprintf("Error al copiar el directorio '%s': %v", entryName, err))
						} else {
							fmt.Printf("[Depuración] copyDirectoryInEXT2: Subdirectorio '%s' (indirecto) copiado exitosamente\n", entryName)
						}
					default:
						copyErrors = append(copyErrors, fmt.Sprintf("Tipo de inodo desconocido para '%s': %d", entryName, entryInode.Itype))
					}
				}
			}
		}
	}

	// Mostrar errores de copia si hubo algunos, pero continuar con la operación
	if len(copyErrors) > 0 {
		fmt.Printf("WARNING: Algunos elementos no se pudieron copiar en '%s':\n", sourceName)
		for _, errMsg := range copyErrors {
			fmt.Printf("  - %s\n", errMsg)
		}
	}

	// La copia se considera exitosa incluso si algunos archivos no pudieron copiarse (copia parcial)
	return nil
}

func addEntryToDirectoryEXT2(file *os.File, ext2fs *EstructuraExt2P2.EXT2FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, dirInodeNum int32, name string, inodeNum int32) bool {
	// Verificación de seguridad para directorio
	if dirInode.Itype != 0 {
		fmt.Printf("Error crítico: El inodo #%d no es un directorio (Itype=%d). Operación cancelada.\n", dirInodeNum, dirInode.Itype)
		return false
	}

	// IMPORTANTE: Leer el inodo directamente del disco para asegurar que tenemos la información más actualizada
	// Esto es crucial para evitar trabajar con una copia desactualizada del inodo
	currentDirInode := &EstructuraarchivocarpetaP2.Inodo{}
	err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+dirInodeNum*ext2fs.SuperBlock.S_inode_s), currentDirInode)
	if err != nil {
		fmt.Printf("Error: No se pudo leer el inodo #%d del directorio: %v\n", dirInodeNum, err)
		return false
	}

	// Usar el inodo más actualizado
	dirInode = currentDirInode

	// Verificación de seguridad para el inodo del archivo
	if inodeNum == 0 {
		fmt.Printf("Error crítico: Se intentó agregar una entrada para el inodo raíz (#0). Operación cancelada.\n")
		return false
	}

	for i := 0; i < 12; i++ {
		if dirInode.Iblock[i] == -1 {
			// Crear un nuevo bloque de directorio si es necesario
			dirBlock := BloquesP2.NewBloqueCarpeta()
			blockNum, err := EstructuraExt2P2.FindFreeBlockEXT2(file, ext2fs)
			if err != nil {
				fmt.Printf("Error: no se pudo encontrar un bloque libre para el directorio: %v\n", err)
				return false
			}
			if blockNum == -1 {
				fmt.Printf("Error: no hay bloques libres disponibles para el directorio\n")
				return false
			}

			dirInode.Iblock[i] = blockNum
			fmt.Printf("[Depuración] addEntryToDirectoryEXT2: Asignando nuevo bloque #%d al directorio (Iblock[%d])\n", blockNum, i)
			err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+blockNum*ext2fs.SuperBlock.S_block_s), dirBlock)
			if err != nil {
				fmt.Printf("Error: no se pudo escribir el nuevo bloque de directorio: %v\n", err)
				return false
			}
		}

		dirBlock := &BloquesP2.BloqueCarpeta{}
		err := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+dirInode.Iblock[i]*ext2fs.SuperBlock.S_block_s), dirBlock)
		if err != nil {
			fmt.Printf("Error: no se pudo leer el bloque de directorio: %v\n", err)
			return false
		}

		if dirBlock.AddEntry(name, inodeNum) {
			fmt.Printf("[Depuración] addEntryToDirectoryEXT2: Agregando entrada '%s' (inodo #%d) en bloque #%d del directorio\n", name, inodeNum, dirInode.Iblock[i])
			err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+dirInode.Iblock[i]*ext2fs.SuperBlock.S_block_s), dirBlock)
			if err != nil {
				fmt.Printf("Error: no se pudo escribir el bloque de directorio actualizado: %v\n", err)
				return false
			}
			// Depuración: leer y mostrar el tipo del inodo recién agregado
			tmpInode := &EstructuraarchivocarpetaP2.Inodo{}
			err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+inodeNum*ext2fs.SuperBlock.S_inode_s), tmpInode)
			if err == nil {
				fmt.Printf("[Depuración] addEntryToDirectoryEXT2: Inodo agregado #%d, Itype=%d (0=dir,1=archivo)\n", inodeNum, tmpInode.Itype)
			}
			// Depuración: mostrar el tipo del directorio después de agregar
			tmpDirInode := &EstructuraarchivocarpetaP2.Inodo{}
			err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+dirInodeNum*ext2fs.SuperBlock.S_inode_s), tmpDirInode)
			if err == nil {
				fmt.Printf("[Depuración] addEntryToDirectoryEXT2: Directorio después de agregar, inodo #%d, Itype=%d (0=dir,1=archivo)\n", dirInodeNum, tmpDirInode.Itype)

				// Verificar que la entrada se agregó correctamente
				verifyBlock := &BloquesP2.BloqueCarpeta{}
				err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+dirInode.Iblock[i]*ext2fs.SuperBlock.S_block_s), verifyBlock)
				if err == nil {
					fmt.Printf("[Depuración] addEntryToDirectoryEXT2: Verificando contenido del bloque del directorio:\n")
					for j := 0; j < 4; j++ {
						if verifyBlock.B_content[j].B_inodo != -1 {
							entryName := strings.TrimRight(string(verifyBlock.B_content[j].B_name[:]), "\x00")
							fmt.Printf("[Depuración]   - Entrada: '%s' -> inodo #%d\n", entryName, verifyBlock.B_content[j].B_inodo)

							// Verificar también el tipo del inodo enlazado
							entryInode := &EstructuraarchivocarpetaP2.Inodo{}
							if utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+verifyBlock.B_content[j].B_inodo*ext2fs.SuperBlock.S_inode_s), entryInode) == nil {
								fmt.Printf("[Depuración]     (tipo=%d)\n", entryInode.Itype)
							}
						}
					}
				}
			}

			// Asegurarse que el tipo de inodo siga siendo directorio (0)
			// Esto es una protección adicional para evitar cambio accidental de tipo
			dirInode.Itype = 0

			// Actualizar el inodo del directorio en el disco
			err = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_inode_start+dirInodeNum*ext2fs.SuperBlock.S_inode_s), dirInode)
			if err != nil {
				fmt.Printf("Error: no se pudo actualizar el inodo del directorio #%d en el disco: %v\n", dirInodeNum, err)
				return false
			}

			// Verificar que el directorio de origen no sea modificado accidentalmente
			// Este es un paso crítico para prevenir corrupción en la estructura de directorios
			if dirInodeNum != 0 { // Si no es el directorio raíz, verificar su padre
				// Leer el bloque de directorio actual para verificar entradas "." y ".."
				rootInode := &EstructuraarchivocarpetaP2.Inodo{}
				rootErr := utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start), rootInode)
				if rootErr == nil && rootInode.Iblock[0] != -1 {
					// Verificar que el directorio raíz sigue teniendo sus entradas correctas
					rootBlock := &BloquesP2.BloqueCarpeta{}
					rootErr = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_block_start+rootInode.Iblock[0]*ext2fs.SuperBlock.S_block_s), rootBlock)
					if rootErr == nil {
						// Verificar que las entradas "." y ".." del directorio raíz son correctas
						if rootBlock.B_content[0].B_inodo != 0 || rootBlock.B_content[1].B_inodo != 0 {
							fmt.Printf("[Depuración] Corrigiendo entradas del directorio raíz\n")
							rootBlock.B_content[0].B_inodo = 0 // "." apunta a sí mismo
							copy(rootBlock.B_content[0].B_name[:], ".")
							rootBlock.B_content[1].B_inodo = 0 // ".." también apunta a sí mismo (es la raíz)
							copy(rootBlock.B_content[1].B_name[:], "..")

							_ = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_block_start+rootInode.Iblock[0]*ext2fs.SuperBlock.S_block_s), rootBlock)
						}
					}
				}
			}

			// Verificación adicional: leer nuevamente el inodo para confirmar que sigue siendo un directorio
			verifyInode := &EstructuraarchivocarpetaP2.Inodo{}
			err = utilitiesP2.ReadStruct(file, int64(ext2fs.SuperBlock.S_inode_start+dirInodeNum*ext2fs.SuperBlock.S_inode_s), verifyInode)
			if err == nil && verifyInode.Itype != 0 {
				fmt.Printf("Error crítico: El inodo #%d ha cambiado de tipo después de la escritura. Corrigiendo...\n", dirInodeNum)
				// Corregir y reescribir
				verifyInode.Itype = 0
				_ = utilitiesP2.WriteStruct(file, int64(ext2fs.SuperBlock.S_inode_start+dirInodeNum*ext2fs.SuperBlock.S_inode_s), verifyInode)
			}

			return true
		}
	}

	fmt.Printf("Error: no hay espacio disponible en los bloques del directorio para agregar la entrada '%s'\n", name)
	return false
}

func copyFileInEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, sourceInode *EstructuraarchivocarpetaP2.Inodo, sourceName string, destInode *EstructuraarchivocarpetaP2.Inodo, destName string, _ *sessionP2.UserSession, destInodeNum int32) error {

	if sourceInode.Itype != 1 {
		fmt.Printf("Error: El inodo fuente no es un archivo (Itype=%d)\n", sourceInode.Itype)
		return nil
	}

	canRead, err := sessionP2.CheckPermissions(sourceInode, sessionP2.READ)
	if err != nil || !canRead {
		fmt.Printf("Error: no tiene permisos de lectura para el archivo '%s': %v\n", sourceName, err)
		return err
	}

	canWrite, err := sessionP2.CheckPermissions(destInode, sessionP2.WRITE)
	if err != nil || !canWrite {
		fmt.Printf("Error: no tiene permisos de escritura en el directorio destino para '%s': %v\n", destName, err)
		return err
	}

	newInodeNum, err := EstructuraExt3P2.FindFreeInodeEXT3(file, ext3fs)
	if err != nil {
		fmt.Printf("Error: no se pudo encontrar un inodo libre: %v\n", err)
		return err
	}

	if newInodeNum == 0 {
		fmt.Printf("Error: no hay inodos libres disponibles para el archivo destino\n")

		newInodeNum, err = EstructuraExt3P2.FindFreeInodeEXT3(file, ext3fs)
		if err != nil || newInodeNum <= 0 {
			fmt.Printf("Error crítico: no se pudo encontrar un inodo libre para el archivo destino\n")
			return fmt.Errorf("no inodos libres disponibles para el archivo destino")
		}
	}

	newInode := &EstructuraarchivocarpetaP2.Inodo{}
	*newInode = *sourceInode
	newInode.Iblock = [16]int32{}
	for i := 0; i < 16; i++ {
		newInode.Iblock[i] = -1
	}
	newInode.Is = sourceInode.Is

	blocks := sourceInode.Iblock

	for i := 0; i < 12; i++ {
		if blocks[i] == -1 {
			continue
		}
		sourceBlock := &BloquesP2.BloqueArchivo{}
		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+blocks[i]*ext3fs.SuperBlock.S_block_s), sourceBlock)
		if err != nil {
			fmt.Printf("Error: no se pudo leer el bloque fuente %d: %v\n", i, err)
			return err
		}
		destBlockNum, err := findFreeBlockEXT3(file, ext3fs)
		if err != nil {
			fmt.Printf("Error: no se pudo encontrar un bloque libre para el destino: %v\n", err)
			return err
		}
		if destBlockNum == -1 {
			fmt.Printf("Error: no hay bloques libres disponibles para el destino\n")
			return fmt.Errorf("no bloques libres disponibles para el destino")
		}
		err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+destBlockNum*ext3fs.SuperBlock.S_block_s), sourceBlock)
		if err != nil {
			fmt.Printf("Error: no se pudo escribir el bloque destino %d: %v\n", i, err)
			return err
		}
		newInode.Iblock[i] = destBlockNum
	}

	if blocks[12] != -1 {
		sourcePtrBlock := &BloquesP2.BloqueApuntador{}
		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+blocks[12]*ext3fs.SuperBlock.S_block_s), sourcePtrBlock)
		if err != nil {
			fmt.Printf("Error: no se pudo leer el bloque apuntador fuente (simple): %v\n", err)
			return err
		}
		destPtrBlockNum, err := findFreeBlockEXT3(file, ext3fs)
		if err != nil || destPtrBlockNum == -1 {
			fmt.Printf("Error: no se pudo encontrar un bloque libre para el bloque apuntador destino (simple)\n")
			return err
		}
		newPtrBlock := &BloquesP2.BloqueApuntador{}
		for i, ptr := range sourcePtrBlock.B_pointers {
			if ptr == -1 {
				newPtrBlock.B_pointers[i] = -1
				continue
			}
			sourceDataBlock := &BloquesP2.BloqueArchivo{}
			err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+ptr*ext3fs.SuperBlock.S_block_s), sourceDataBlock)
			if err != nil {
				fmt.Printf("Error: no se pudo leer el bloque de datos fuente (indirecto simple): %v\n", err)
				return err
			}
			destDataBlockNum, err := findFreeBlockEXT3(file, ext3fs)
			if err != nil || destDataBlockNum == -1 {
				fmt.Printf("Error: no se pudo encontrar un bloque libre para el bloque de datos destino (indirecto simple)\n")
				return err
			}
			err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+destDataBlockNum*ext3fs.SuperBlock.S_block_s), sourceDataBlock)
			if err != nil {
				fmt.Printf("Error: no se pudo escribir el bloque de datos destino (indirecto simple): %v\n", err)
				return err
			}
			newPtrBlock.B_pointers[i] = destDataBlockNum
		}
		err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+destPtrBlockNum*ext3fs.SuperBlock.S_block_s), newPtrBlock)
		if err != nil {
			fmt.Printf("Error: no se pudo escribir el bloque apuntador destino (simple): %v\n", err)
			return err
		}
		newInode.Iblock[12] = destPtrBlockNum
	}

	// los demás bloques indirectos si esque son necesarios

	err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_inode_start+newInodeNum*ext3fs.SuperBlock.S_inode_s), newInode)
	if err != nil {
		fmt.Printf("Error: no se pudo escribir el nuevo inodo en el disco: %v\n", err)
		return err
	}

	tmpInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+newInodeNum*ext3fs.SuperBlock.S_inode_s), tmpInode)
	if err == nil {
		fmt.Printf("[Depuración] copyFileInEXT3: Después de escribir inodo #%d, Itype=%d (0=dir,1=archivo)\n", newInodeNum, tmpInode.Itype)
	}

	if !addEntryToDirectoryEXT3(file, ext3fs, destInode, destInodeNum, destName, newInodeNum) {
		fmt.Printf("Error: No se pudo agregar la entrada '%s' al directorio destino\n", destName)
		return nil
	}

	return nil

}

func copyDirectoryInEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, sourceInode *EstructuraarchivocarpetaP2.Inodo, sourceName string, destInode *EstructuraarchivocarpetaP2.Inodo, destName string, sessionInfo *sessionP2.UserSession) error {
	if sourceInode.Itype != 0 {
		fmt.Printf("Error: El inodo fuente no es un directorio (Itype=%d)\n", sourceInode.Itype)
		return nil
	}

	canRead, err := sessionP2.CheckPermissions(sourceInode, sessionP2.READ)
	if err != nil || !canRead {
		fmt.Printf("Error: no tiene permisos de lectura para el directorio '%s': %v\n", sourceName, err)
		return err
	}

	canWrite, err := sessionP2.CheckPermissions(destInode, sessionP2.WRITE)
	if err != nil || !canWrite {
		fmt.Printf("Error: no tiene permisos de escritura en el directorio destino para '%s': %v\n", destName, err)
		return err
	}

	newInodeNum, err := EstructuraExt3P2.FindFreeInodeEXT3(file, ext3fs)
	if err != nil {
		fmt.Printf("Error: no se pudo encontrar un inodo libre para el directorio destino: %v\n", err)
		return err
	}

	newInode := &EstructuraarchivocarpetaP2.Inodo{}
	*newInode = *sourceInode
	newInode.Iblock = [16]int32{}
	for i := 0; i < 16; i++ {
		newInode.Iblock[i] = -1
	}
	newInode.Is = 0

	newDirBlock := BloquesP2.NewBloqueCarpeta()

	newBlockNum, err := findFreeBlockEXT3(file, ext3fs)
	if err != nil {
		return fmt.Errorf("error al buscar bloque libre: %v", err)
	}

	newInode.Iblock[0] = newBlockNum

	newDirBlock.AddEntry(".", newInodeNum)
	newDirBlock.AddEntry("..", int32(0))

	err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+newBlockNum*ext3fs.SuperBlock.S_block_s), newDirBlock)
	if err != nil {
		fmt.Printf("Error: no se pudo escribir el bloque del nuevo directorio: %v\n", err)
		return err
	}

	fmt.Printf("[Depuración] copyDirectoryInEXT3: Antes de escribir inodo destino #%d, Itype=%d (0=dir,1=archivo)\n", newInodeNum, newInode.Itype)
	err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_inode_start+newInodeNum*ext3fs.SuperBlock.S_inode_s), newInode)
	if err != nil {
		fmt.Printf("Error: no se pudo escribir el nuevo inodo en el disco: %v\n", err)
		return err
	}
	tmpInode := &EstructuraarchivocarpetaP2.Inodo{}
	err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+newInodeNum*ext3fs.SuperBlock.S_inode_s), tmpInode)
	if err == nil {
		fmt.Printf("[Depuración] copyDirectoryInEXT3: Después de escribir inodo #%d, Itype=%d (0=dir,1=archivo)\n", newInodeNum, tmpInode.Itype)
	}

	var copyErrors []string

	blocks := sourceInode.Iblock

	for i := 0; i < 12; i++ {
		if sourceInode.Iblock[i] == -1 {
			continue
		}

		sourceBlock := &BloquesP2.BloqueCarpeta{}
		err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+sourceInode.Iblock[i]*ext3fs.SuperBlock.S_block_s), sourceBlock)
		if err != nil {
			copyErrors = append(copyErrors, fmt.Sprintf("No se pudo leer el bloque %d del directorio fuente: %v", i, err))
			continue
		}

		for _, entry := range sourceBlock.B_content {
			entryName := strings.TrimRight(string(entry.B_name[:]), "\x00")
			if entry.B_inodo == -1 || entryName == "." || entryName == ".." {
				continue
			}

			entryInode := &EstructuraarchivocarpetaP2.Inodo{}
			err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+entry.B_inodo*ext3fs.SuperBlock.S_inode_s), entryInode)
			if err != nil {
				copyErrors = append(copyErrors, fmt.Sprintf("No se pudo leer el inodo de '%s': %v", entryName, err))
				continue
			}

			canReadEntry, err := sessionP2.CheckPermissions(entryInode, sessionP2.READ)
			if err != nil || !canReadEntry {
				copyErrors = append(copyErrors, fmt.Sprintf("No se pudo copiar '%s': falta permiso de lectura", entryName))
				continue
			}

			switch entryInode.Itype {
			case 1:
				err = copyFileInEXT3(file, ext3fs, entryInode, entryName, newInode, entryName, sessionInfo, newInodeNum)
				if err != nil {
					copyErrors = append(copyErrors, fmt.Sprintf("Error al copiar el archivo '%s': %v", entryName, err))
				}
			case 0:
				err = copyDirectoryInEXT3(file, ext3fs, entryInode, entryName, newInode, entryName, nil)
				if err != nil {
					copyErrors = append(copyErrors, fmt.Sprintf("Error al copiar el directorio '%s': %v", entryName, err))
				}
			default:
				copyErrors = append(copyErrors, fmt.Sprintf("Tipo de inodo desconocido para '%s': %d", entryName, entryInode.Itype))
			}
		}
	}

	if blocks[12] != -1 {
		sourceBlock := &BloquesP2.BloqueApuntador{}
		err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+sourceInode.Iblock[12]*ext3fs.SuperBlock.S_block_s), sourceBlock)
		if err != nil {
			copyErrors = append(copyErrors, fmt.Sprintf("No se pudo leer el bloque apuntador simple del directorio fuente: %v", err))
		} else {
			for _, ptr := range sourceBlock.B_pointers {
				if ptr == -1 {
					continue
				}
				indirectBlock := &BloquesP2.BloqueCarpeta{}
				err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+ptr*ext3fs.SuperBlock.S_block_s), indirectBlock)
				if err != nil {
					copyErrors = append(copyErrors, fmt.Sprintf("No se pudo leer el bloque de directorio indirecto: %v", err))
					continue
				}
				for _, entry := range indirectBlock.B_content {
					entryName := strings.TrimRight(string(entry.B_name[:]), "\x00")
					if entry.B_inodo == -1 || entryName == "." || entryName == ".." {
						continue
					}

					entryInode := &EstructuraarchivocarpetaP2.Inodo{}
					err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+entry.B_inodo*ext3fs.SuperBlock.S_inode_s), entryInode)
					if err != nil {
						copyErrors = append(copyErrors, fmt.Sprintf("No se pudo leer el inodo de '%s': %v", entryName, err))
						continue
					}

					canReadEntry, err := sessionP2.CheckPermissions(entryInode, sessionP2.READ)
					if err != nil || !canReadEntry {
						copyErrors = append(copyErrors, fmt.Sprintf("No se pudo copiar '%s': falta permiso de lectura", entryName))
						continue
					}

					switch entryInode.Itype {
					case 1:
						err = copyFileInEXT3(file, ext3fs, entryInode, entryName, newInode, entryName, sessionInfo, newInodeNum)
						if err != nil {
							copyErrors = append(copyErrors, fmt.Sprintf("Error al copiar el archivo '%s': %v", entryName, err))
						}
					case 0:
						err = copyDirectoryInEXT3(file, ext3fs, entryInode, entryName, newInode, entryName, nil)
						if err != nil {
							copyErrors = append(copyErrors, fmt.Sprintf("Error al copiar el directorio '%s': %v", entryName, err))
						}
					default:
						copyErrors = append(copyErrors, fmt.Sprintf("Tipo de inodo desconocido para '%s': %d", entryName, entryInode.Itype))
					}
				}
			}
		}
	}

	if len(copyErrors) > 0 {
		fmt.Printf("WARNING: Algunos elementos no se pudieron copiar en '%s':\n", sourceName)
		for _, errMsg := range copyErrors {
			fmt.Printf("  - %s\n", errMsg)
		}
	}

	return nil
}

func addEntryToDirectoryEXT3(file *os.File, ext3fs *EstructuraExt3P2.EXT3FileSystem, dirInode *EstructuraarchivocarpetaP2.Inodo, dirInodeNum int32, name string, inodeNum int32) bool {
	if dirInode.Itype != 0 {
		fmt.Printf("Error crítico: El inodo #%d no es un directorio (Itype=%d). Operación cancelada.\n", dirInodeNum, dirInode.Itype)
		return false
	}

	if inodeNum == 0 {
		fmt.Printf("Error crítico: Se intentó agregar una entrada para el inodo raíz (#0). Operación cancelada.\n")
		return false
	}

	for i := 0; i < 12; i++ {
		if dirInode.Iblock[i] == -1 {
			dirBlock := BloquesP2.NewBloqueCarpeta()
			blockNum, err := findFreeBlockEXT3(file, ext3fs)
			if err != nil {
				fmt.Printf("Error: no se pudo encontrar un bloque libre para el directorio: %v\n", err)
				return false
			}
			if blockNum == -1 {
				fmt.Printf("Error: no hay bloques libres disponibles para el directorio\n")
				return false
			}

			dirInode.Iblock[i] = blockNum
			fmt.Printf("[Depuración] addEntryToDirectoryEXT3: Asignando nuevo bloque #%d al directorio (Iblock[%d])\n", blockNum, i)
			err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+blockNum*ext3fs.SuperBlock.S_block_s), dirBlock)
			if err != nil {
				fmt.Printf("Error: no se pudo escribir el nuevo bloque de directorio: %v\n", err)
				return false
			}
		}

		dirBlock := &BloquesP2.BloqueCarpeta{}
		err := utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_block_start+dirInode.Iblock[i]*ext3fs.SuperBlock.S_block_s), dirBlock)
		if err != nil {
			fmt.Printf("Error: no se pudo leer el bloque de directorio: %v\n", err)
			return false
		}

		if dirBlock.AddEntry(name, inodeNum) {
			fmt.Printf("[Depuración] addEntryToDirectoryEXT3: Agregando entrada '%s' (inodo #%d) en bloque #%d del directorio\n", name, inodeNum, dirInode.Iblock[i])
			err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_block_start+dirInode.Iblock[i]*ext3fs.SuperBlock.S_block_s), dirBlock)
			if err != nil {
				fmt.Printf("Error: no se pudo escribir el bloque de directorio actualizado: %v\n", err)
				return false
			}
			tmpInode := &EstructuraarchivocarpetaP2.Inodo{}
			err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+inodeNum*ext3fs.SuperBlock.S_inode_s), tmpInode)
			if err == nil {
				fmt.Printf("[Depuración] addEntryToDirectoryEXT3: Inodo agregado #%d, Itype=%d (0=dir,1=archivo)\n", inodeNum, tmpInode.Itype)
			}
			tmpDirInode := &EstructuraarchivocarpetaP2.Inodo{}
			err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+dirInodeNum*ext3fs.SuperBlock.S_inode_s), tmpDirInode)
			if err == nil {
				fmt.Printf("[Depuración] addEntryToDirectoryEXT3: Directorio después de agregar, inodo #%d, Itype=%d (0=dir,1=archivo)\n", dirInodeNum, tmpDirInode.Itype)
			}
			dirInode.Itype = 0

			err = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_inode_start+dirInodeNum*ext3fs.SuperBlock.S_inode_s), dirInode)
			if err != nil {
				fmt.Printf("Error: no se pudo actualizar el inodo del directorio #%d en el disco: %v\n", dirInodeNum, err)
				return false
			}

			verifyInode := &EstructuraarchivocarpetaP2.Inodo{}
			err = utilitiesP2.ReadStruct(file, int64(ext3fs.SuperBlock.S_inode_start+dirInodeNum*ext3fs.SuperBlock.S_inode_s), verifyInode)
			if err == nil && verifyInode.Itype != 0 {
				fmt.Printf("Error crítico: El inodo #%d ha cambiado de tipo después de la escritura. Corrigiendo...\n", dirInodeNum)
				verifyInode.Itype = 0
				_ = utilitiesP2.WriteStruct(file, int64(ext3fs.SuperBlock.S_inode_start+dirInodeNum*ext3fs.SuperBlock.S_inode_s), verifyInode)
			}

			return true
		}
	}

	fmt.Printf("Error: no hay espacio disponible en los bloques del directorio para agregar la entrada '%s'\n", name)
	return false
}
