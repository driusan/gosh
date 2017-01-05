MDFILES=README.md Tokenization.md TabCompletion.md Piping.md \
	BackgroundProcesses.md Environment.md BackgroundProcessesRevisited.md \
	TabCompletionRevisited.md

all: $(MDFILES)
	lmt $(MDFILES)
	go fmt .
	go build . 