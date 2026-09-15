import asyncio
import json
import websockets

SERVER_URL = "ws://localhost:8000/agent/ws"


async def receiver(ws):
    """Prints anything the server sends, in the background, so pings/acks
    show up immediately without blocking your typing."""
    try:
        async for message in ws:
            print(f"\n<<< {message}\n> ", end="", flush=True)
    except websockets.exceptions.ConnectionClosed as e:
        print(f"\n[connection closed: code={e.code} reason={e.reason}]")


async def main():
    async with websockets.connect(SERVER_URL) as ws:
        print(f"connected to {SERVER_URL}")
        recv_task = asyncio.create_task(receiver(ws))

        print("Paste a JSON message and hit enter to send it. Ctrl+C to quit.")
        loop = asyncio.get_event_loop()
        try:
            while True:
                line = await loop.run_in_executor(None, input, "> ")
                line = line.strip()
                if not line:
                    continue
                try:
                    json.loads(line)  # just validate it's real JSON before sending
                except json.JSONDecodeError as e:
                    print(f"[not valid JSON: {e}]")
                    continue
                await ws.send(line)
        except (KeyboardInterrupt, EOFError):
            pass
        finally:
            recv_task.cancel()


if __name__ == "__main__":
    asyncio.run(main())