build:
	cd proto && bash _generate.sh && cd .. && go build -o ~/go/bin/go-plugin .