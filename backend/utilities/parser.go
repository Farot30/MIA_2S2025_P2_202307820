package utilities

import (
	"fmt"
	"regexp"
	"strings"
)

// ParseCommand parsea una línea de comando y extrae el comando y sus parámetros
func ParseCommand(input string) []string {
	// Remover comentarios
	if strings.Contains(input, "#") {
		input = strings.Split(input, "#")[0]
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return []string{}
	}

	// Regex para parsear comandos con parámetros que pueden tener comillas
	re := regexp.MustCompile(`[^\s"'=]+(?:=(?:"[^"]*"|'[^']*'|[^\s]+))?|"[^"]*"|'[^']*'`)
	matches := re.FindAllString(input, -1)

	var result []string
	for _, match := range matches {
		result = append(result, match)
	}

	return result
}

func HasParam(params map[string]string, key string) bool {
	_, exists := params[key]
	return exists
}

// ParseParameters parsea los parámetros de un comando en formato -param=value
func ParseParameters(args []string) map[string]string {
	params := make(map[string]string)

	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			// Buscar el primer '=' para separar parámetro y valor
			equalIndex := strings.Index(arg[1:], "=")
			if equalIndex != -1 {
				// Ajustar índice porque buscamos desde arg[1:]
				equalIndex = equalIndex + 1

				paramName := strings.ToLower(arg[1:equalIndex])
				paramValue := arg[equalIndex+1:]

				// REMOVER COMILLAS AQUÍ si existen
				if (strings.HasPrefix(paramValue, "\"") && strings.HasSuffix(paramValue, "\"")) ||
					(strings.HasPrefix(paramValue, "'") && strings.HasSuffix(paramValue, "'")) {
					paramValue = paramValue[1 : len(paramValue)-1]
				}

				params[paramName] = paramValue
			} else {
				// Parámetro sin valor (como -r, -p)
				paramName := strings.ToLower(arg[1:])
				params[paramName] = ""
			}
		}
	}

	return params
}

// ValidateRequiredParams valida que los parámetros requeridos estén presentes
func ValidateRequiredParams(params map[string]string, required []string) error {
	for _, req := range required {
		if _, exists := params[req]; !exists {
			return fmt.Errorf("parámetro requerido '%s' no encontrado", req)
		}
	}
	return nil
}

func GetParamString(params map[string]string, key string) string {
	value := params[key]
	// Doble limpieza por si las comillas no se removieron antes
	return strings.Trim(value, "\"'")
}

func GetParamStringWithSpaces(params map[string]string, key string) string {
	return params[key] // Ya debería estar sin comillas gracias a ParseParameters
}

// ValidateAllowedParams verifica que no hayan parámetros no permitidos
// allowed: lista de nombres (sin '-') permitidos para el comando, ej: []string{"size","unit","path","fit"}
func ValidateAllowedParams(params map[string]string, allowed []string) error {
	allowedSet := make(map[string]bool)
	for _, a := range allowed {
		allowedSet[strings.ToLower(a)] = true
	}

	var unknown []string
	for p := range params {
		if !allowedSet[p] {
			unknown = append(unknown, p)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("parámetro(s) no reconocidos: %s", strings.Join(unknown, ", "))
	}
	return nil
}
