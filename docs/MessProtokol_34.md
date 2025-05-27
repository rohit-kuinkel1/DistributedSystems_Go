Before: Gateway generates data → HTTP → Server → RPC → Database
After:  Sensors → MQTT → Gateway → HTTP → Server → RPC → Database

```bash
make build
make run-mqtt-system
```
Visit http://localhost:8080 to see the web interface
You should see data from sensors like temp-1, humid-1, etc.
Check logs to see MQTT messages being published/received