# dynamo-session-store

dynamo-session-store is HTTP session store with DynamoDB for Gorilla Toolkit


## Features
- [x] Secure Cookie
- [x] Compatible with [AWS SDK for Go V2](https://aws.github.io/aws-sdk-go-v2/)

## Preparation

You have to create a table having a partition key named `id`

`id` is typed by string.

## Examples
```go
import (
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/itto-ki/dynamo-session-store/store"
)

cfg, err := config.LoadDefaultConfig(context.Background())
if err != nil {
    panic(err.Error())
}

sessionStore := store.NewStore("table-name", cfg, []byte("your-secret-key"))

// Get a session
session, err = sessionStore.Get(req, "session-key")
if err != nil {
    // error
}

// Add a value.
session.Values["foo"] = "bar"

// Save
if err = session.Save(req, rsp); err != nil {
    // error
}

// Delete session
session.Options.MaxAge = -1
if err = session.Save(req, rsp); err != nil {
    // error
}
```

## License
MIT licensed. See the LICENSE file for details.