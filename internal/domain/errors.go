package domain

import "errors"

// Sentinels wrapped by adapters with %w; cancellation uses context errors instead.
var (
	ErrInvalidPath       = errors.New("ruta inválida")
	ErrInvalidArgument   = errors.New("argumento inválido")
	ErrNotFound          = errors.New("no encontrado")
	ErrConflict          = errors.New("conflicto")
	ErrBinary            = errors.New("archivo binario")
	ErrTooLarge          = errors.New("contenido demasiado grande")
	ErrPermissionDenied  = errors.New("permiso denegado")
	ErrNotRepository     = errors.New("no es un repositorio git")
	ErrGitIndexLocked    = errors.New("el índice de git está bloqueado")
	ErrNoRemote          = errors.New("no hay remoto configurado")
	ErrDockerUnavailable = errors.New("docker no está disponible")
)
