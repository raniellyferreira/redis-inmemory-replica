package policy

// EstimateCostBytes retorna um custo simples baseado no tamanho do payload.
// Implementações futuras podem considerar metadados, cardinalidade, etc.
func EstimateCostBytes(b []byte) int64 {
	if b == nil {
		return 0
	}
	return int64(len(b))
}
