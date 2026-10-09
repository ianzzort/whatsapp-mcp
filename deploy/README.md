# Despliegue del bridge de WhatsApp en Linux

## Compilar

El bridge usa `go-sqlite3`, que requiere CGO y un compilador C.

```bash
sudo apt-get install -y gcc libc6-dev
cd whatsapp-bridge
CGO_ENABLED=1 go build -o whatsapp-client .
```

## Instalar

```bash
sudo useradd --system --home /opt/wa-bridge --shell /usr/sbin/nologin wabridge
sudo mkdir -p /opt/wa-bridge/store
sudo cp whatsapp-client /opt/wa-bridge/
sudo chown -R wabridge:wabridge /opt/wa-bridge
sudo cp deploy/wa-bridge.service /etc/systemd/system/
sudo install -m 600 -o root -g root /dev/null /etc/wa-bridge.env
sudo systemctl daemon-reload
sudo systemctl enable --now wa-bridge
```

El servicio usa `WorkingDirectory=/opt/wa-bridge`; las bases viven en `/opt/wa-bridge/store/`.

## Variables (`/etc/wa-bridge.env`)

```
WHATSAPP_BRIDGE_BIND=127.0.0.1
WHATSAPP_BRIDGE_PORT=8590
WHATSAPP_BRIDGE_TOKEN=<token largo y aleatorio>
WHATSAPP_ALERT_WEBHOOK=https://ejemplo.com/hooks/whatsapp-alerta
```

- `WHATSAPP_BRIDGE_TOKEN`: si existe, todos los `/api/*` exigen `Authorization: Bearer <token>`. Sin ella el bridge arranca con una advertencia y sin autenticación.
- `WHATSAPP_ALERT_WEBHOOK`: recibe un POST JSON `{event, at, message}` cuando la sesión se cierra (`logged_out`) o pasa más de 5 minutos desconectado (`disconnected`).
- Mantén el bind en `127.0.0.1`; si el bot corre en otra máquina, usa un túnel o una VPN.

## Vincular el QR en el servidor

Con la sesión vacía (`store/whatsapp.db` inexistente) el bridge imprime el QR en el log:

```bash
sudo journalctl -u wa-bridge -f | grep QR_RAW
```

Copia el valor después de `QR_RAW:` y genera la imagen en tu equipo (por ejemplo `qrencode -t ANSIUTF8 '<valor>'`), luego escanéalo desde WhatsApp > Dispositivos vinculados. El código rota cada ~20 segundos; usa el último que aparezca. Si no lo vinculas en 3 minutos el proceso termina y systemd lo reinicia con un QR nuevo.

## Respaldo

Respalda `/opt/wa-bridge/store/` completo (`whatsapp.db` es la sesión; `messages.db` el historial). Usa WAL: para una copia consistente usa `sqlite3 messages.db ".backup respaldo.db"` o detén el servicio antes de copiar. Trata `store/` como un secreto: quien tenga `whatsapp.db` controla la sesión.

## Caducidad de sesión

WhatsApp cierra los dispositivos vinculados que no se conectan al teléfono principal en ~14 días y, en la práctica, la sesión puede caducar cada ~20 días. Abre WhatsApp en el teléfono con regularidad; si llega la alerta `logged_out`, hay que volver a vincular el QR.

## Comprobar

```bash
curl -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8590/api/health
```
