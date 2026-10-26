package app

type asyncCompletion struct {
	key   string
	apply func()
}

func (a *App) runAsync(key string, operation func() func()) bool {
	if a.asyncJobs == nil {
		a.asyncJobs = map[string]bool{}
		a.asyncResults = make(chan asyncCompletion, 128)
	}
	if a.asyncJobs[key] {
		return false
	}
	a.asyncJobs[key] = true
	win := a.win
	go func() {
		apply := operation()
		a.asyncResults <- asyncCompletion{key: key, apply: apply}
		if win != nil {
			win.Invalidate()
		}
	}()
	return true
}

func (a *App) asyncBusy(key string) bool {
	return a.asyncJobs[key]
}

func (a *App) drainAsync() {
	for {
		select {
		case result := <-a.asyncResults:
			delete(a.asyncJobs, result.key)
			if result.apply != nil {
				result.apply()
			}
		default:
			return
		}
	}
}
