# What is this?

`immich-archive-import` is heavily opinionated tool that recursiely looks through the given directory for archive files (*.zip, *.7z, *.rar). When an archive file is found, it creates a new album on the given [immich](https://immich.app) with the relative path as the album name. Then it uploads [asset files](https://docs.immich.app/features/supported-formats/) inside to Immich and lastly assign the asset files to the newly created album.

If there's another album with the name of said relative path, the archive will be skipped.

# Setup

`immich-archive-import` can be installed using `go install` commands.

```sh
$ go install github.com/wutipong/immich-archive-import@latest
```

After that it requires a configuration file. This file is located at `
`/.immich-archive-import/config.yml`. Please create it first otherwise the tool won't start. The content of the file should looks like this.

```yaml
default:
    immich_url: http://localhost:2283/ # replace this with immich server url
    immich_api_key: VtoFituqTaLrgxkX4t3xteZ7lAGgfSDHV0Aqkp4M # and this with immich API Key, please see below.
```

The API Key can be obtained by open **⚙️Account Settings** and Open **API Keys section**. Create a new API key to include *asset, album, and albumAsset* permission (I haven't tried the fine-grain permission settings yet, please bear with me.). Make sure to copy the key after the creation, then use it on the configuration file as stated above.

# Running

To start the tool, run the following command.

```sh
$ immich-archive-import --dir=path-of-archive
```

Then it will start the process of uploading. Beaware that if somehow the process fails, there is no fallback. The uploaded assets will stay inside Immich unless manually deleted. The album that created by the failed attempt will stay as well, so you might have to manually delete the album.

# Profile

`immich-archive-import` supports using different configurations in each run using profile. A new profile can be added to the `config.yaml` by adding another key to the file. For example:

```yaml
default:
    immich_url: http://localhost:2283/
    immich_api_key: VtoFituqTaLrgxkX4t3xteZ7lAGgfSDHV0Aqkp4M
   
# this is an additional dev profile
dev:
    immich_url: http://localhost:2283/
    immich_api_key: VtoFituqTaLrgxkX4t3xteZ7lAGgfSDHV0Aqkp4M
    log_level: debug
```

By default, `immich-archive-import` use `default` profile. To use a different profile, use `--profile=<profile>` parameter. For example.

```sh
$ immich-archive-import --profile=dev --dir=path-of-archive
```

You can use different profile for many different purpose, like enable debugging, connect to a different server, or import album with different user, etc.

# Development

For the ease of development purpose, the project source files include self-contained immich's `docker-compose.yaml` with the volume section modified so it does not write to the folder directly. Use this file to start a localhost instance.

Of course PR and issues are welcome.