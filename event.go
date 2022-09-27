package master

var (
	events []func()
)

func RegisterExitHandler(event func()) {
	events = append(events, event)
}
