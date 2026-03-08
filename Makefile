GOBUILD = CGO_ENABLED=0 GOOS=linux go build -o ./go_app
INSTALL_DIR = /usr/local/bin

.PHONY: all case-converter check-folder-size find-content find-everything replace-text api-stress-test clean

all: case-converter check-folder-size find-content find-everything replace-text api-stress-test

case-converter:
	cd case-converter && $(GOBUILD)
	sudo mv case-converter/go_app $(INSTALL_DIR)/c

check-folder-size:
	cd check-folder-size && $(GOBUILD)
	sudo mv check-folder-size/go_app $(INSTALL_DIR)/check-folder-size

find-content:
	cd find-content && $(GOBUILD)
	sudo mv find-content/go_app $(INSTALL_DIR)/find-content

find-everything:
	cd find-everything && $(GOBUILD)
	sudo mv find-everything/go_app $(INSTALL_DIR)/find-everything

replace-text:
	cd replace-text && $(GOBUILD)
	sudo mv replace-text/go_app $(INSTALL_DIR)/replace-text

api-stress-test:
	cd api-stress-test && $(GOBUILD)
	sudo mv api-stress-test/go_app $(INSTALL_DIR)/api-stress-test

clean:
	rm -f */go_app
