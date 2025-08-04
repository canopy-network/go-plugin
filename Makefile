build:
	cd proto && bash _generate.sh && go build -o ~/go/bin/go-plugin .