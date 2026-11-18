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
	ErrNoChanges         = errors.New("sin cambios preparados")
	ErrGitUnavailable    = errors.New("git no está disponible")
	ErrNoRemote          = errors.New("no hay remoto configurado")
	ErrDirtyWorktree     = errors.New("hay cambios sin guardar en el árbol de trabajo")
	ErrNoUpstream        = errors.New("la rama no tiene upstream")
	ErrNonFastForward    = errors.New("el remoto tiene commits que no están en local")
	ErrAuthRequired      = errors.New("el remoto pide autenticación")
	ErrRemoteUnreachable = errors.New("no se pudo alcanzar el remoto")
	ErrMergeInProgress   = errors.New("hay un merge en curso")
	ErrNoMergeInProgress = errors.New("no hay ningún merge en curso")
	ErrDockerUnavailable = errors.New("docker no está disponible")
)
