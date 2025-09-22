package session

import (
	estructuraArchivoCarpeta "MIA_2S2025_P1_202307820/estructuraArchivoCarpeta"
	"fmt"
)

// PermissionType representa los tipos de permisos
type PermissionType int

const (
	READ    PermissionType = 4 // r--
	WRITE   PermissionType = 2 // -w-
	EXECUTE PermissionType = 1 // --x
)

// UserCategory representa la categoría del usuario respecto al archivo
type UserCategory int

const (
	OWNER UserCategory = iota // User (Propietario)
	GROUP                     // Group (Mismo grupo)
	OTHER                     // Other (Otros usuarios)
)

// IsRootUser verifica si el usuario actual es root
func IsRootUser() bool {
	session := GetCurrentSession()
	if session == nil {
		return false
	}

	// Root puede ser identificado por username "root" o UID 1
	return session.Username == "root" || session.UID == 1
}

// GetUserCategory determina en qué categoría está el usuario actual respecto a un archivo/carpeta
func GetUserCategory(fileOwnerUID, fileOwnerGID int32) UserCategory {
	session := GetCurrentSession()
	if session == nil {
		return OTHER // Sin sesión = otros usuarios
	}

	// Si es el propietario del archivo
	if session.UID == fileOwnerUID {
		return OWNER
	}

	// Si pertenece al mismo grupo que el propietario del archivo
	if session.GID == fileOwnerGID {
		return GROUP
	}

	// En cualquier otro caso
	return OTHER
}

// CheckPermissions verifica si el usuario actual tiene permisos para una operación específica
func CheckPermissions(inode *estructuraArchivoCarpeta.Inodo, operation PermissionType) (bool, error) {
	// USUARIO ROOT SIEMPRE TIENE TODOS LOS PERMISOS (777)
	if IsRootUser() {
		fmt.Printf("Usuario ROOT detectado - Permisos COMPLETOS (777) otorgados\n")
		return true, nil
	}

	// Verificar que hay una sesión activa
	session := GetCurrentSession()
	if session == nil {
		return false, fmt.Errorf("no hay sesión activa")
	}

	// Obtener permisos del archivo y propietarios
	filePermissions := inode.GetIperm()
	fileOwnerUID := inode.GetIuid()
	fileOwnerGID := inode.GetIgid()

	// Determinar la categoría del usuario actual
	userCategory := GetUserCategory(fileOwnerUID, fileOwnerGID)

	// Obtener los permisos correspondientes según la categoría
	var relevantPermissions byte
	var categoryName string

	switch userCategory {
	case OWNER:
		relevantPermissions = filePermissions[0] // User permissions
		categoryName = "PROPIETARIO"
	case GROUP:
		relevantPermissions = filePermissions[1] // Group permissions
		categoryName = "GRUPO"
	case OTHER:
		relevantPermissions = filePermissions[2] // Other permissions
		categoryName = "OTROS"
	}

	// Verificar si tiene el permiso específico
	hasPermission := (relevantPermissions & byte(operation)) != 0

	// DEBUG: Información detallada
	fmt.Printf("VERIFICACIÓN DE PERMISOS:\n")
	fmt.Printf("   Archivo UID: %d, GID: %d\n", fileOwnerUID, fileOwnerGID)
	fmt.Printf("   Usuario actual: %s (UID: %d, GID: %d)\n", session.Username, session.UID, session.GID)
	fmt.Printf("   Categoría: %s\n", categoryName)
	fmt.Printf("   Permisos archivo: %d%d%d (UGO)\n", filePermissions[0], filePermissions[1], filePermissions[2])
	fmt.Printf("   Permisos relevantes: %d\n", relevantPermissions)
	fmt.Printf("   Operación solicitada: %s (%d)\n", getOperationName(operation), operation)
	fmt.Printf("   Resultado: %v\n", hasPermission)
	fmt.Printf("=====================================\n")

	if !hasPermission {
		return false, fmt.Errorf("sin permisos: usuario '%s' (%s) no tiene permisos de %s sobre el archivo",
			session.Username, categoryName, getOperationName(operation))
	}

	return true, nil
}

// RequirePermissions verifica permisos y devuelve error si no los tiene
func RequirePermissions(inode *estructuraArchivoCarpeta.Inodo, operation PermissionType) error {
	hasPermission, err := CheckPermissions(inode, operation)
	if err != nil {
		return err
	}
	if !hasPermission {
		return fmt.Errorf("permisos insuficientes para la operación")
	}
	return nil
}

// CheckMultiplePermissions verifica múltiples permisos a la vez
func CheckMultiplePermissions(inode *estructuraArchivoCarpeta.Inodo, operations ...PermissionType) (bool, error) {
	for _, operation := range operations {
		hasPermission, err := CheckPermissions(inode, operation)
		if err != nil {
			return false, err
		}
		if !hasPermission {
			return false, fmt.Errorf("sin permisos para operación: %s", getOperationName(operation))
		}
	}
	return true, nil
}

// getOperationName convierte el tipo de operación a string para debug
func getOperationName(operation PermissionType) string {
	switch operation {
	case READ:
		return "LECTURA"
	case WRITE:
		return "ESCRITURA"
	case EXECUTE:
		return "EJECUCIÓN"
	default:
		return "DESCONOCIDA"
	}
}

// GetPermissionsString retorna una representación legible de los permisos
func GetPermissionsString(permissions [3]byte) string {
	result := ""

	for _, perm := range permissions {
		if perm&4 != 0 {
			result += "r"
		} else {
			result += "-"
		}
		if perm&2 != 0 {
			result += "w"
		} else {
			result += "-"
		}
		if perm&1 != 0 {
			result += "x"
		} else {
			result += "-"
		}
	}

	return result
}

// CanRead verifica permisos de lectura
func CanRead(inode *estructuraArchivoCarpeta.Inodo) bool {
	can, _ := CheckPermissions(inode, READ)
	return can
}

// CanWrite verifica permisos de escritura
func CanWrite(inode *estructuraArchivoCarpeta.Inodo) bool {
	can, _ := CheckPermissions(inode, WRITE)
	return can
}

// CanExecute verifica permisos de ejecución
func CanExecute(inode *estructuraArchivoCarpeta.Inodo) bool {
	can, _ := CheckPermissions(inode, EXECUTE)
	return can
}

// ShowPermissionsInfo muestra información detallada de permisos
func ShowPermissionsInfo(inode *estructuraArchivoCarpeta.Inodo) {
	session := GetCurrentSession()
	if session == nil {
		fmt.Println("No hay sesión activa")
		return
	}

	permissions := inode.GetIperm()
	fileOwnerUID := inode.GetIuid()
	fileOwnerGID := inode.GetIgid()
	userCategory := GetUserCategory(fileOwnerUID, fileOwnerGID)

	fmt.Printf("\nINFORMACIÓN DE PERMISOS:\n")
	fmt.Printf("=======================================\n")
	fmt.Printf("Usuario actual: %s (UID: %d, GID: %d)\n", session.Username, session.UID, session.GID)
	fmt.Printf("Archivo/Carpeta: UID: %d, GID: %d\n", fileOwnerUID, fileOwnerGID)
	fmt.Printf("Permisos: %s (%d%d%d)\n", GetPermissionsString(permissions), permissions[0], permissions[1], permissions[2])

	var categoryStr string
	switch userCategory {
	case OWNER:
		categoryStr = "PROPIETARIO (U)"
	case GROUP:
		categoryStr = "GRUPO (G)"
	case OTHER:
		categoryStr = "OTROS (O)"
	}
	fmt.Printf("📂 Categoría del usuario: %s\n", categoryStr)

	if IsRootUser() {
		fmt.Printf("USUARIO ROOT - Permisos COMPLETOS (777)\n")
	} else {
		fmt.Printf("Lectura: %v\n", CanRead(inode))
		fmt.Printf("Escritura: %v\n", CanWrite(inode))
		fmt.Printf("Ejecución: %v\n", CanExecute(inode))
	}
	fmt.Printf("=======================================\n\n")
}

// RequireRootUser verifica que el usuario actual sea root
func RequireRootUser() error {
	if !IsRootUser() {
		session := GetCurrentSession()
		if session == nil {
			return fmt.Errorf("no hay sesión activa")
		}
		return fmt.Errorf("solo el usuario root puede ejecutar este comando. Usuario actual: %s", session.Username)
	}
	return nil
}

// RequireReadAccess verifica permisos de lectura
func RequireReadAccess(inode *estructuraArchivoCarpeta.Inodo) error {
	return RequirePermissions(inode, READ)
}

// RequireWriteAccess verifica permisos de escritura
func RequireWriteAccess(inode *estructuraArchivoCarpeta.Inodo) error {
	return RequirePermissions(inode, WRITE)
}

// RequireExecuteAccess verifica permisos de ejecución
func RequireExecuteAccess(inode *estructuraArchivoCarpeta.Inodo) error {
	return RequirePermissions(inode, EXECUTE)
}
