package policy

// EvictionPolicy define ganchos para atualização de estado interno e seleção de chaves para evicção.
type EvictionPolicy interface {
	// OnGet é chamado em acessos (para contagem/frequência).
	OnGet(key string)
	// OnSet é chamado em gravações com custo estimado.
	OnSet(key string, cost int64)
	// OnDel é chamado em remoções explícitas.
	OnDel(key string)
	// Evict retorna até 'count' chaves candidatas à remoção.
	Evict(count int) []string
}

// PlaceholderLRU é um placeholder que não executa evicção real.
// Serve apenas para scaffolding nesta fase.
type PlaceholderLRU struct{}

func (p *PlaceholderLRU) OnGet(key string)             {}
func (p *PlaceholderLRU) OnSet(key string, cost int64) {}
func (p *PlaceholderLRU) OnDel(key string)             {}
func (p *PlaceholderLRU) Evict(count int) []string     { return nil }
