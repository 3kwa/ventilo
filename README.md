HTTP POST a single field form `message` to the `broadcast/<channel>`

It will be sent to any WebSocket or Server Sent Event listener on `listen/<channel>`

### publisher

```python
import httpx

httpx.post("http://localhost:8080/broadcast/a/channel", data={"message": "a message"})
```

### listener

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>ws and/or sse</title>
</head>
<body>
    <script>
        // ws
        const ws = new WebSocket('ws://localhost:8080/listen/a/channel');
        ws.onmessage = function(event) {
            console.log('ws:', event.data);
        };
        // sse
        const sse = new EventSource('http://localhost:8080/listen/a/channel');
        sse.onmessage = function(event) {
            console.log('sse:', event.data);
        };
    </script>
</body>
</html>
```
