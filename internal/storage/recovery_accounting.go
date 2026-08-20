package storage

const recoveredSourceHash = "backscroll:recovered"

func isRecoveredSourceHash(hash string) bool {
	return hash == recoveredSourceHash
}
