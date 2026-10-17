package composer

import "sort"

// Severity says whether a rule reports an Error (Docker Compose would
// reject the file, or it is plainly wrong) or a Warning (valid, but risky
// or surprising).
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Rule describes one check komposer performs. IDs are stable: once a rule
// has shipped its ID is never renamed or reused, because scripts, config
// files and `komposer explain` refer to it.
type Rule struct {
	ID       string
	Severity Severity
	// Title is a one-line summary of what the rule flags.
	Title string
	// Why explains the risk, or why Compose rejects it.
	Why string
	// Fix says what to change.
	Fix string
}

// rules is the registry of every rule validate.go and security.go can
// emit. rules_test.go checks it against the source so the two can't drift.
var rules = []Rule{
	// ---- errors ------------------------------------------------------
	{"no-services", SeverityError,
		"The file defines no services.",
		"A Compose file with an empty or missing 'services:' section has nothing to run.",
		"Add at least one service under 'services:'."},
	{"duplicate-service", SeverityError,
		"Two services have the same name.",
		"Service names are the keys of the 'services:' map, so they must be unique; a file with duplicates is invalid YAML for Compose.",
		"Give each service a unique name."},
	{"depends-on-unknown-service", SeverityError,
		"depends_on points at a service that doesn't exist.",
		"Compose refuses to start a stack whose depends_on references an undefined service.",
		"Fix the name, or define the missing service."},
	{"depends-on-healthy-without-healthcheck", SeverityError,
		"depends_on waits for 'service_healthy' on a service with no healthcheck.",
		"The condition can never be satisfied without a healthcheck on the dependency, so Compose refuses to start the stack.",
		"Add a healthcheck to the dependency, or use 'condition: service_started'."},
	{"depends-on-cycle", SeverityError,
		"Services depend on each other in a cycle.",
		"There is no valid start order for a dependency cycle, so Compose refuses to start the stack.",
		"Break the cycle: remove one of the depends_on links, or move the shared part into its own service."},
	{"image-or-build-required", SeverityError,
		"A service has neither 'image' nor 'build'.",
		"Compose needs to know where the container's image comes from.",
		"Set 'image', 'build', or both ('image' then names what 'build' produces)."},
	{"invalid-port", SeverityError,
		"A ports entry isn't valid short syntax.",
		"Compose can't parse the entry, so it refuses to start the stack. Valid forms include '8080', '8080:80', '127.0.0.1:8080:80', '[::1]:8080:80', '8000-8010:8000-8010' and an optional '/tcp', '/udp' or '/sctp' suffix.",
		"Rewrite the entry in one of the valid forms, or use the long syntax (target/published/host_ip)."},
	{"invalid-environment-entry", SeverityError,
		"An environment list entry isn't in KEY=value form.",
		"Entries of an environment list must look like KEY=value (or a bare KEY to pass a host variable through).",
		"Write the entry as KEY=value."},
	{"invalid-restart-policy", SeverityError,
		"'restart' isn't one of no, always, on-failure, unless-stopped.",
		"Compose rejects unknown restart policies.",
		"Use one of: no, always, on-failure, unless-stopped."},
	{"healthcheck-test-required", SeverityError,
		"A healthcheck has no test command.",
		"Without 'test' the healthcheck has nothing to run.",
		"Add a 'test' (for example [\"CMD\", \"curl\", \"-f\", \"http://localhost/\"])."},
	{"invalid-healthcheck-duration", SeverityError,
		"A healthcheck interval, timeout, start_period or start_interval isn't a valid duration.",
		"Durations look like '30s', '1m30s' or '500ms'; Compose rejects anything else.",
		"Use a duration with a unit, such as '30s'."},

	// ---- warnings: images ---------------------------------------------
	{"image-untagged", SeverityWarning,
		"An image has no tag.",
		"Docker defaults to ':latest', which can pull a different image on every 'docker compose pull' with no change to the file to explain why.",
		"Pin an explicit version, for example 'nginx:1.27'."},
	{"image-latest", SeverityWarning,
		"An image is pinned to ':latest'.",
		"':latest' moves whenever the publisher pushes, so the same file can run different software from one day to the next.",
		"Pin an explicit version tag, or an @sha256 digest for fully reproducible deploys."},
	{"image-rolling-tag", SeverityWarning,
		"An image uses a rolling tag such as stable, edge, main or nightly.",
		"Tags like these are re-pointed over time, which makes deployments non-reproducible in the same way ':latest' does.",
		"Pin a specific version, or an @sha256 digest."},

	// ---- warnings: network ---------------------------------------------
	{"port-all-interfaces", SeverityWarning,
		"A port is published on every network interface.",
		"Without an explicit host IP Docker binds 0.0.0.0 (and, depending on setup, ::), exposing the service to the whole network — and Docker's published ports bypass common host firewall rules.",
		"Bind to loopback ('127.0.0.1:8080:80' or '[::1]:8080:80') unless the service must be reachable from other machines."},
	{"host-network", SeverityWarning,
		"A service uses 'network_mode: host'.",
		"The container shares the host's network stack, removing network isolation between it and the host.",
		"Use a normal network and publish only the ports you need, unless the service genuinely requires host networking."},

	// ---- warnings: privileges --------------------------------------------
	{"privileged", SeverityWarning,
		"A service runs with 'privileged: true'.",
		"A privileged container has access to all host devices and nearly all kernel capabilities, so a compromise is close to a compromise of the host.",
		"Remove it, or grant only what is needed with 'cap_add' and 'devices'."},
	{"user-root", SeverityWarning,
		"A service explicitly runs as root.",
		"Running as root inside the container widens the damage of any escape or file-permission mistake.",
		"Set 'user' to an unprivileged UID:GID if the image supports it."},
	{"dangerous-capability", SeverityWarning,
		"'cap_add' grants a broad Linux capability (ALL, SYS_ADMIN, NET_ADMIN, SYS_PTRACE, SYS_MODULE).",
		"These capabilities unlock device, kernel, network-configuration or process-tracing access that is roughly comparable to 'privileged' for what they cover.",
		"Add only the narrow capabilities the service needs."},
	{"host-pid", SeverityWarning,
		"A service uses 'pid: host'.",
		"The container can see and signal every process on the host.",
		"Remove it unless the service is a process monitor that needs it."},
	{"host-ipc", SeverityWarning,
		"A service uses 'ipc: host'.",
		"The container shares the host's shared memory and message queues, so it can read and interfere with other processes' IPC.",
		"Use the default private IPC namespace, or 'ipc: shareable' / 'service:<name>' to share deliberately."},
	{"host-cgroup", SeverityWarning,
		"A service uses 'cgroup: host'.",
		"The container shares the host's cgroup namespace and can see every cgroup on the machine.",
		"Use the default 'cgroup: private' unless the service manages cgroups."},
	{"host-userns", SeverityWarning,
		"A service uses 'userns_mode: host'.",
		"It disables user-namespace remapping, so root in the container is root on the host.",
		"Remove it unless the service cannot run with remapped users."},
	{"security-opt-unconfined", SeverityWarning,
		"'security_opt' sets an 'unconfined' profile (seccomp, AppArmor, ...).",
		"An unconfined profile removes a sandbox layer, for example the seccomp filter that blocks many dangerous system calls.",
		"Use the default profile, or a custom one that opens only what the service needs."},
	{"no-new-privileges-disabled", SeverityWarning,
		"'security_opt' sets no-new-privileges:false.",
		"This explicitly allows processes in the container to gain privileges through setuid binaries.",
		"Remove it, or set 'no-new-privileges:true'."},

	// ---- warnings: mounts -----------------------------------------------
	{"docker-socket", SeverityWarning,
		"A service mounts the Docker socket.",
		"Whoever controls the container can drive the host's Docker daemon, which is equivalent to root on the host (and read-only mounts don't change that).",
		"Avoid it; if a tool really needs it, use a socket proxy that exposes only the API calls required."},
	{"sensitive-host-mount", SeverityWarning,
		"A sensitive host directory is bind-mounted read-write.",
		"Directories such as /, /etc, /root, /proc, /sys, /dev and /var/lib/docker control how the host boots, authenticates and runs containers; write access from a container lets a compromise reach them.",
		"Mount it read-only (':ro'), or mount just the files or subdirectory the service needs."},

	// ---- warnings: secrets ----------------------------------------------
	{"secret-empty", SeverityWarning,
		"A variable that looks like a secret has an empty value.",
		"An empty password is as risky as a hardcoded one, and some images disable authentication entirely when it's blank.",
		"Set it through env_file, Docker secrets or ${VAR} substitution, or remove the key if it's unused."},
	{"secret-hardcoded", SeverityWarning,
		"A variable that looks like a secret has its value written in the file.",
		"The value ends up in version control and in every copy of the file.",
		"Move it to an env_file or Compose secret, or substitute it with ${VAR}. komposer can do this conversion for you in the editor."},

	// ---- warnings: databases ---------------------------------------------
	{"database-no-volume", SeverityWarning,
		"A database image has no volume.",
		"All data is lost whenever the container is removed or recreated, which 'docker compose up' does routinely.",
		"Mount a volume at the database's data directory."},
	{"database-no-healthcheck", SeverityWarning,
		"A database image has no healthcheck.",
		"Without one, anything that depends_on this service can only wait for the process to start, not for the database to accept connections.",
		"Add a healthcheck (komposer suggests one for known images) and use 'condition: service_healthy'."},
	{"postgres-no-password", SeverityWarning,
		"Postgres is published on the network with no password set.",
		"Anyone who can reach the port has full database access.",
		"Set POSTGRES_PASSWORD (or POSTGRES_PASSWORD_FILE), or don't publish the port."},
	{"postgres-trust-auth", SeverityWarning,
		"Postgres runs with POSTGRES_HOST_AUTH_METHOD=trust.",
		"Every client that can reach the database connects without a password.",
		"Remove it and set a password instead."},
	{"mongo-no-auth", SeverityWarning,
		"MongoDB is published on the network with no authentication configured.",
		"Without MONGO_INITDB_ROOT_USERNAME/PASSWORD or --auth, MongoDB starts with access control off, so anyone who can reach the port has full access.",
		"Configure a root user (or --auth), or don't publish the port."},
	{"mysql-empty-password", SeverityWarning,
		"MySQL/MariaDB allows an empty root password.",
		"MYSQL_ALLOW_EMPTY_PASSWORD and MARIADB_ALLOW_EMPTY_ROOT_PASSWORD let root connect with no password.",
		"Set a root password instead."},
}

// Rules returns every rule komposer knows, sorted by severity (errors
// first) and then by ID. The slice is a copy.
func Rules() []Rule {
	out := append([]Rule(nil), rules...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity == SeverityError
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// LookupRule returns the rule with the given ID.
func LookupRule(id string) (Rule, bool) {
	for _, r := range rules {
		if r.ID == id {
			return r, true
		}
	}
	return Rule{}, false
}
