import asyncio
import aiohttp
import time
import random
from collections import defaultdict

# Configuration
LB_URL = "http://localhost:3030"
TOTAL_REQUESTS = 100
CONCURRENCY = 20
SLOW_RATIO = 0.3  # 30% slow requests

# ANSI colors
RESET = "\033[0m"
RED = "\033[31m"
GREEN = "\033[32m"
YELLOW = "\033[33m"
BLUE = "\033[34m"
CYAN = "\033[36m"

# Stats
backend_stats = defaultdict(lambda: {"fast": 0, "slow": 0})
start_time = time.time()

async def fetch(session, url, is_slow):
    try:
        async with session.get(url) as response:
            text = await response.text()
            # Extract hostname from response "Hello from backend! I am running on <hostname>"
            # or "Slept 5 seconds on <hostname>"
            parts = text.strip().split()
            hostname = parts[-1] if parts else "unknown"
            
            if is_slow:
                backend_stats[hostname]["slow"] += 1
                print(f"{RED}SLOW  (5s){RESET} -> {hostname}")
            else:
                backend_stats[hostname]["fast"] += 1
                print(f"{GREEN}FAST{RESET}       -> {hostname}")
                
            return hostname
    except Exception as e:
        print(f"{YELLOW}ERROR{RESET}: {e}")
        return None

async def worker(queue, session):
    while True:
        is_slow = await queue.get()
        endpoint = "/sleep" if is_slow else "/"
        await fetch(session, f"{LB_URL}{endpoint}", is_slow)
        queue.task_done()


async def poll_stats(session, interval=0.5):
    """Poll /stats and print a compact status line with conn counts."""
    while True:
        try:
            async with session.get(f"{LB_URL}/stats") as resp:
                if resp.status == 200:
                    data = await resp.json()
                    parts = []
                    for b in data:
                        url = b.get("url", "?")
                        conn = b.get("conn_count", 0)
                        parts.append(f"{url.split('//')[-1]}:{conn}")
                    line = " | ".join(parts)
                    print(f"{BLUE}STATS{RESET} {line}")
        except Exception:
            print(f"{YELLOW}STATS ERR{RESET}")
        await asyncio.sleep(interval)

async def main():
    print(f"{CYAN}Starting Visual Load Test...{RESET}")
    print(f"Target: {LB_URL}")
    print(f"Requests: {TOTAL_REQUESTS} (Mix: {int(SLOW_RATIO*100)}% slow)")
    print("-" * 40)

    queue = asyncio.Queue()
    
    # Fill queue
    for _ in range(TOTAL_REQUESTS):
        is_slow = random.random() < SLOW_RATIO
        queue.put_nowait(is_slow)

    async with aiohttp.ClientSession() as session:
        workers = []
        # start stats poller
        poller = asyncio.create_task(poll_stats(session, interval=0.5))
        for _ in range(CONCURRENCY):
            task = asyncio.create_task(worker(queue, session))
            workers.append(task)

        await queue.join()
        
        for task in workers:
            task.cancel()

        poller.cancel()

    print("-" * 40)
    print(f"{CYAN}Test Completed in {time.time() - start_time:.2f}s{RESET}")
    print("\nDistribution Report:")
    print(f"{'Backend':<20} | {'Fast Req':<10} | {'Slow Req':<10} | {'Total':<10}")
    print("-" * 60)
    
    for host, counts in backend_stats.items():
        total = counts['fast'] + counts['slow']
        print(f"{host:<20} | {counts['fast']:<10} | {counts['slow']:<10} | {total:<10}")

if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        pass
