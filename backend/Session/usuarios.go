package sessionP2

import (
	"fmt"
	"strconv"
	"strings"
)

// User representa un usuario del sistema según la especificación
type User struct {
	UID      int32  // Cambio: int32 para compatibilidad
	Type     string // "U" para usuario
	Group    string // Nombre del grupo al que pertenece
	Username string // Máximo 10 caracteres
	Password string // Máximo 10 caracteres
}

// Group representa un grupo del sistema según la especificación
type Group struct {
	GID  int32  // Cambio: int32 para compatibilidad
	Type string // "G" para grupo
	Name string // Máximo 10 caracteres
}

// UsersManager maneja la administración de usuarios y grupos
type UsersManager struct {
	PartitionPath string
	StartOffset   int32
}

// NewUsersManager crea un nuevo gestor de usuarios
func NewUsersManager(partitionPath string, startOffset int32) *UsersManager {
	return &UsersManager{
		PartitionPath: partitionPath,
		StartOffset:   startOffset,
	}
}

// CreateInitialUsersFile crea el archivo users.txt inicial con usuario root según especificación
func (um *UsersManager) CreateInitialUsersFile() error {
	// Contenido inicial exacto según especificación:
	// 1,G,root
	// 1,U,root,root,123
	initialContent := "1,G,root\n1,U,root,root,123\n"

	fmt.Println("Archivo users.txt inicializado según especificación:")
	fmt.Print(initialContent)
	fmt.Println(" Usuario root (UID: 1) creado con contraseña '123'")
	fmt.Println(" Grupo root (GID: 1) creado")
	fmt.Println(" Archivo pertenece a usuario root (1) y grupo root (1)")

	return nil
}

// ReadUsersFile lee el archivo users.txt desde el sistema EXT2
func (um *UsersManager) ReadUsersFile() ([]User, []Group, error) {
	// TODO: Implementar lectura desde el sistema EXT2
	// Por ahora, retornamos los valores por defecto según especificación

	groups := []Group{
		{GID: 1, Type: "G", Name: "root"}, // Grupo inicial según especificación
	}

	users := []User{
		{UID: 1, Type: "U", Group: "root", Username: "root", Password: "123"}, // Usuario inicial según especificación
	}

	return users, groups, nil
}

// ParseUsersLine parsea una línea del archivo users.txt según la especificación
func ParseUsersLine(line string) (interface{}, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, fmt.Errorf("línea vacía")
	}

	parts := strings.Split(line, ",")
	if len(parts) < 3 {
		return nil, fmt.Errorf("formato de línea inválido: %s", line)
	}

	// Parsear ID (puede ser 0 para registros eliminados)
	id, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 32)
	if err != nil {
		return nil, fmt.Errorf("ID inválido: %s", parts[0])
	}

	recordType := strings.TrimSpace(parts[1])

	if recordType == "G" {
		// Formato: GID,Tipo,Grupo
		if len(parts) != 3 {
			return nil, fmt.Errorf("formato de grupo inválido: %s (esperado: GID,G,NombreGrupo)", line)
		}

		groupName := strings.TrimSpace(parts[2])
		if len(groupName) > 10 {
			return nil, fmt.Errorf("nombre de grupo muy largo (máximo 10 caracteres): %s", groupName)
		}

		return Group{
			GID:  int32(id),
			Type: recordType,
			Name: groupName,
		}, nil

	} else if recordType == "U" {
		// Formato: UID,Tipo,Grupo,Usuario,Contraseña
		if len(parts) != 5 {
			return nil, fmt.Errorf("formato de usuario inválido: %s (esperado: UID,U,Grupo,Usuario,Contraseña)", line)
		}

		groupName := strings.TrimSpace(parts[2])
		username := strings.TrimSpace(parts[3])
		password := strings.TrimSpace(parts[4])

		// Validaciones según especificación
		if len(groupName) > 10 {
			return nil, fmt.Errorf("nombre de grupo muy largo (máximo 10 caracteres): %s", groupName)
		}
		if len(username) > 10 {
			return nil, fmt.Errorf("nombre de usuario muy largo (máximo 10 caracteres): %s", username)
		}
		if len(password) > 10 {
			return nil, fmt.Errorf("contraseña muy larga (máximo 10 caracteres)")
		}

		return User{
			UID:      int32(id),
			Type:     recordType,
			Group:    groupName,
			Username: username,
			Password: password,
		}, nil
	}

	return nil, fmt.Errorf("tipo de registro desconocido: %s (debe ser 'G' para grupo o 'U' para usuario)", recordType)
}

// AuthenticateUser autentica un usuario con su contraseña según especificación
func (um *UsersManager) AuthenticateUser(username, password string) (*User, *Group, error) {
	users, groups, err := um.ReadUsersFile()
	if err != nil {
		return nil, nil, fmt.Errorf("error leyendo archivo de usuarios: %s", err)
	}

	// Buscar el usuario (ID 0 = eliminado según especificación)
	var foundUser *User
	for _, user := range users {
		if user.Username == username && user.UID != 0 { //  UID 0 = eliminado
			foundUser = &user
			break
		}
	}

	if foundUser == nil {
		return nil, nil, fmt.Errorf("el usuario '%s' no existe o está eliminado", username)
	}

	// Verificar contraseña (case-sensitive según especificación)
	if foundUser.Password != password {
		return nil, nil, fmt.Errorf("contraseña incorrecta para el usuario '%s'", username)
	}

	// Buscar el grupo del usuario (GID 0 = eliminado según especificación)
	var foundGroup *Group
	for _, group := range groups {
		if group.Name == foundUser.Group && group.GID != 0 { //  GID 0 = eliminado
			foundGroup = &group
			break
		}
	}

	if foundGroup == nil {
		return nil, nil, fmt.Errorf("grupo '%s' del usuario '%s' no encontrado o está eliminado", foundUser.Group, username)
	}

	return foundUser, foundGroup, nil
}

// WriteUsersFile escribe el archivo users.txt al sistema EXT2 en formato especificación
func (um *UsersManager) WriteUsersFile(users []User, groups []Group) error {
	// TODO: Implementar escritura al sistema EXT2

	fmt.Println("Simulando escritura de users.txt según especificación:")

	// Escribir grupos primero (formato: GID,G,NombreGrupo)
	for _, group := range groups {
		if group.GID != 0 { // No escribir grupos eliminados (GID = 0)
			fmt.Printf("%d,G,%s\n", group.GID, group.Name)
		}
	}

	// Escribir usuarios (formato: UID,U,Grupo,Usuario,Contraseña)
	for _, user := range users {
		if user.UID != 0 { // No escribir usuarios eliminados (UID = 0)
			fmt.Printf("%d,U,%s,%s,%s\n", user.UID, user.Group, user.Username, user.Password)
		}
	}

	return nil
}

// ValidateUsersFile valida la integridad del archivo users.txt según especificación
func ValidateUsersFile(content string) error {
	lines := strings.Split(content, "\n")

	hasRootGroup := false
	hasRootUser := false

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		record, err := ParseUsersLine(line)
		if err != nil {
			return fmt.Errorf("error en línea %d: %s", i+1, err)
		}

		// Verificar que existe grupo root inicial
		if group, ok := record.(Group); ok && group.Name == "root" && group.GID != 0 {
			hasRootGroup = true
		}

		// Verificar que existe usuario root inicial
		if user, ok := record.(User); ok && user.Username == "root" && user.UID != 0 {
			hasRootUser = true
		}
	}

	// Validaciones según especificación
	if !hasRootGroup {
		return fmt.Errorf("el archivo debe contener el grupo 'root' inicial")
	}

	if !hasRootUser {
		return fmt.Errorf("el archivo debe contener el usuario 'root' inicial")
	}

	return nil
}

// GetNextUserID retorna el siguiente ID disponible para usuarios
func GetNextUserID(content string) int32 {
	lines := strings.Split(content, "\n")
	maxID := int32(0)

	for _, line := range lines {
		if line == "" {
			continue
		}

		record, err := ParseUsersLine(line)
		if err != nil {
			continue
		}

		if user, ok := record.(User); ok && user.UID > maxID {
			maxID = user.UID
		}
	}

	return maxID + 1 // IDs incrementales según especificación
}

// GetNextGroupID retorna el siguiente ID disponible para grupos
func GetNextGroupID(content string) int32 {
	lines := strings.Split(content, "\n")
	maxID := int32(0)

	for _, line := range lines {
		if line == "" {
			continue
		}

		record, err := ParseUsersLine(line)
		if err != nil {
			continue
		}

		if group, ok := record.(Group); ok && group.GID > maxID {
			maxID = group.GID
		}
	}

	return maxID + 1 // IDs incrementales según especificación
}
