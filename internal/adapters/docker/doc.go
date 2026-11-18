// Package docker implements the Docker port on top of the Moby client.
//
// The socket is never configured here: the host comes from the caller or from DOCKER_HOST.
// The "not found" class the daemon answers for an unknown container is not mapped onto a domain
// sentinel because detecting it would need the containerd/errdefs module as a direct dependency.
package docker
