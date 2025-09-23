package sessionP2

import (
	"fmt"
	"sync"
	"time"
)

// UserSession representa una sesión activa de usuario
type UserSession struct {
	UID         int32 // Cambio: int32 para compatibilidad con mkfs
	GID         int32 // Cambio: int32 para compatibilidad con mkfs
	Username    string
	GroupName   string
	PartitionID string
	IsActive    bool
	LoginTime   time.Time // Agregado: tiempo de login
}

var (
	currentSession *UserSession
	sessionMutex   sync.RWMutex
)

// StartSession inicia una nueva sesión de usuario
func StartSession(userID, groupID int32, username, groupName, partitionID string) error { // Cambio: int32
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	if currentSession != nil && currentSession.IsActive {
		return fmt.Errorf("ya existe una sesión activa. Debe hacer LOGOUT primero")
	}

	currentSession = &UserSession{
		UID:         userID,  // Compatible con login.go
		GID:         groupID, // Compatible con login.go
		Username:    username,
		GroupName:   groupName,
		PartitionID: partitionID,
		IsActive:    true,
		LoginTime:   time.Now(), // Para calcular duración
	}

	return nil
}

// EndSession termina la sesión activa
func EndSession() error { // Cambio: retornar error
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	if currentSession == nil || !currentSession.IsActive {
		return fmt.Errorf("no hay sesión activa para cerrar")
	}

	currentSession = nil
	return nil
}

// ForceEndSession fuerza el cierre de sesión
func ForceEndSession() error {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	currentSession = nil
	return nil
}

// GetCurrentSession retorna la sesión activa
func GetCurrentSession() *UserSession {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	if currentSession == nil || !currentSession.IsActive {
		return nil
	}

	return currentSession
}

// IsSessionActive verifica si hay una sesión activa
func IsSessionActive() bool {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	return currentSession != nil && currentSession.IsActive
}

// RequireActiveSession verifica que haya una sesión activa, si no devuelve error
func RequireActiveSession() error {
	if !IsSessionActive() {
		return fmt.Errorf("no existe una sesión activa. Debe hacer LOGIN primero")
	}
	return nil
}

// GetLoginTime retorna el tiempo de inicio de sesión como string FUNCIÓN AGREGADA
func GetLoginTime() string {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	if currentSession == nil || !currentSession.IsActive {
		return "No hay sesión activa"
	}

	return currentSession.LoginTime.Format("2006-01-02 15:04:05")
}

// GetSessionDuration retorna la duración de la sesión actual FUNCIÓN AGREGADA
func GetSessionDuration() string {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	if currentSession == nil || !currentSession.IsActive {
		return "No hay sesión activa"
	}

	duration := time.Since(currentSession.LoginTime)

	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

// GetSessionInfo retorna información de la sesión actual
func GetSessionInfo() string {
	session := GetCurrentSession()
	if session == nil {
		return "No hay sesión activa"
	}

	return fmt.Sprintf("Usuario: %s | Grupo: %s | Partición: %s | UID: %d | GID: %d",
		session.Username, session.GroupName, session.PartitionID, session.UID, session.GID)
}

// GetCurrentUser retorna la información del usuario actual FUNCIÓN ÚTIL AGREGADA
func GetCurrentUser() (int32, int32, string, string, error) {
	session := GetCurrentSession()
	if session == nil {
		return 0, 0, "", "", fmt.Errorf("no hay sesión activa")
	}

	return session.UID, session.GID, session.Username, session.GroupName, nil
}
