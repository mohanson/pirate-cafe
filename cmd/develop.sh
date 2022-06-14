set -ex

if [ ! -d ./bin ]; then
    mkdir bin
fi

if [ ! -f ./bin/pirate.json ]; then
    cp ./res/pirate.json ./bin/pirate.json
fi

go build -o bin github.com/mohanson/pirate-cafe/cmd/pirate
