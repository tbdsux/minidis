module github.com/tbdsux/minidis/node

go 1.26.0

require (
	github.com/gorilla/websocket v1.5.3
	github.com/tbdsux/minidis/voice v0.0.0
)

require (
	github.com/disgoorg/godave v0.2.0 // indirect
	github.com/tbdsux/minidis v0.0.0
	github.com/thomas-vilte/dave-go v0.5.1 // indirect
	github.com/thomas-vilte/mls-go v1.6.0 // indirect
	golang.org/x/crypto v0.59.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
)

replace github.com/tbdsux/minidis/voice => ../voice

replace github.com/tbdsux/minidis => ../
