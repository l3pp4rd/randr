PREFIX  = $(HOME)/.local
BINDIR  = $(PREFIX)/bin
UNITDIR = $(HOME)/.config/systemd/user

all: randr

randr: main.go
	go build -o randr .

install: randr
	install -d $(BINDIR) $(UNITDIR)
	install -m 755 randr $(BINDIR)/randr
	install -m 644 randr.service $(UNITDIR)/randr.service
	systemctl --user daemon-reload
	systemctl --user enable --now randr.service

uninstall:
	systemctl --user disable --now randr.service || true
	rm -f $(BINDIR)/randr $(UNITDIR)/randr.service
	systemctl --user daemon-reload

status:
	systemctl --user status randr.service

logs:
	journalctl --user -u randr.service -f

clean:
	rm -f randr

.PHONY: all install uninstall status logs clean
