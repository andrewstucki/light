# Light - quick 'n' dirty ngrok clone

This is a stripped down clone of ngrok for setting 
up reverse tunnels to local environments.

## Installation

```bash
go install github.com/andrewstucki/light@latest
```

### Running a Server

An example `docker-compose` manifest is in the [deployment](./deployment) directory.

The simple way I currently run this is to get a $5 DigitalOcean droplet, toss a `.env` file onto the host with my desired `HOST`, `EMAIL`, and `TOKEN` values, and then run via `docker-compose up -d`.

You'll need to create two DNS records for this to work properly, a wildcard for all subdomains of your chosen `HOST` and a record for the bare domain, both pointing to the public IP of the droplet.

### Running the Client

Drop a config file at `~/.light.toml` with your `HOST` and `TOKEN` values like:

```toml
server="https://proxy.my.domain"
token="some-token-here"
```

You should then be able to test everything out (assuming the domain `proxy.my.domain`):

In one terminal:

```bash
light -p 8082 -i test -- python3 -m http.server 8082
```

And in another:

```bash
curl https://test.proxy.my.domain
```