package policy

// AdmissionPolicy decide se um item deve ser admitido no cache.
// Implementações futuras podem usar TinyLFU [Unverified].
type AdmissionPolicy interface {
	// Allow retorna true se o item pode ser admitido.
	Allow(key string, cost int64) bool
	// Record permite alimentar contadores/estatísticas de admissão.
	Record(key string, cost int64)
}

// NoopAdmission aceita sempre.
type NoopAdmission struct{}

func (NoopAdmission) Allow(key string, cost int64) bool { return true }
func (NoopAdmission) Record(key string, cost int64)     {}
