# Freelancer

## REST API

### Client

Client is an individual or organization that post job on exchange

To create new client:

```HTTP
POST /client
```

Payload:

```JSON
{
    "email":"tt@org.com",
    "balance":2000000, // amount in cents
}
```

Response:

```HTTP
HTTP 200

{"id":"{client_id"}
```

To retrieve client info:

```HTTP
GET /client/{id}
```

Response:

```HTTP
HTTP 200

{
    "id":"{client_id}",
    "balance":200000,
    "email":"tt@org.com"
}
```


### Task

Task is a unit of a job that can be posted on exchanged and performed by Freelancer

#### Create

NOTE: When client creates a task, funds will be locked from client's account

```HTTP
POST /task
```

Payload:

```JSON
{
    "description":"golang app",
    "fee":2000,                     //amount in cents
    "deadline":40000,               //duration in seconds
    "client_id":"client-uuid",
}
```

Response:

```HTTP
HTTP 200

{"id":"{task_id"}
```

#### Update

NOTE: When task's status changes to `closed`, funds will be unlocked and transfered to freelancer's account

```HTTP
PUT /task/{id}
```

Payload:

```JSON
{
    "description":"golang app",
    "fee":2000,
    "deadline":40000,
    "status":"started",
    "freelancer_id":"freelancer-uuid"
}
```
