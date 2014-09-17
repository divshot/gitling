# Gitling Smart HTTP Git Server

Gitling is a smart HTTP git server designed for multi-tenant git contexts. Gitling
allows for making an optional HTTP authentication request to an external URL
in order to verify a push and set its location.

**Note:** Gitling only works for HTTP-based git. If you're looking for custom SSH
git authentication (a la GitHub/Heroku), I wish you luck.

## Installation

If you already have a `go` environment up and running, it's quite simple:

    go get github.com/divshot/gitling
    
If you don't, you'll need to make that happen first. Binary distribution

## Usage

Basic usage is as follows:

    gitling --auth-url https://example.com/auth -p 8080 -r /path/to/root -b /usr/bin/git
    
Just try `gitling -h` if you need help with specific options.

### Authorization Endpoint

If you specify an authorization URL using `-a` or `--auth-url`, Gitling will make
an HTTP `POST` request to the specified URL.The request will have a JSON body
containing details about the request. For instance, if I set my git remote as
`https://open:sesame@example.com/project-name.git` and pushed, the request body
would look like this:

```json
{
  "username":"open",
  "password":"sesame",
  "path":"project-name.git"
}
```

Your response to this POST tells Gitling how to proceed. The first signal is the
HTTP status code you return.

* **204:** Authorization granted, path may be used without modification.
* **200:** Authorization granted, response body as specified below.
* **401:** Authentication failed (user/password combo did not grant access)
* **403:** Authorization failed (user does not have access to repo at this path)
* **50x:** If Gitling receives a 50x, it will print a generic error message.

In addition to HTTP status codes, a `200` response should be accompanied with
a JSON object with one or more of the following keys:

* **path:** the path (relative to your project root) for the git repo.
* **read_only:** if `true`, this user may not write to the repo, only read.
* **skip_create:** don't create a new repository if one does not already exist.

If you don't specify a `path`, the path specified by the user will be used. For
error responses (401, 403) the body of the response will be printed out for the
user to see.