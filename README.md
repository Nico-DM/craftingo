# Craftingo

![code coverage badge](https://github.com/Nico-DM/craftingo/actions/workflows/ci.yml/badge.svg)

## Local Development

Make sure you're on Go version 1.27+.

Create a `.env` file in the root of the project with the following contents:

```bash
PORT="8080"
```

Run the server:

```bash
go build -o craftingo && ./craftingo
```

*This starts the server in non-database mode.* It will serve a simple webpage at `http://localhost:8080`.
