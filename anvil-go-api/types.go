package api

type Window struct {
	Id         int
	GlobalPath string
	Path       string
}

type WindowBody struct {
	Len int
}

type Notification struct {
	WinId  int
	Op     NotificationOp
	Offset int
	Len    int
	Cmd    []string
}

type NotificationOp int

const (
	NotificationOpInsert = iota
	NotificationOpDelete
	NotificationOpExec
)

type ExecuteReq struct {
	Cmd  string
	Args []string
}
