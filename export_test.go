package sri

import "time"

// IntervaloDe expone a las pruebas el intervalo efectivo de consulta.
func IntervaloDe(c *Cliente) time.Duration { return c.cfg.Espera.Intervalo }
