#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-2.0-or-later
"""Forward encrypted informs to a real controller, failing one chosen request.

For a guarded apply with a nonempty delta, request 1 is the normal inform and
request 2 is its post-change controller health check. Set HTTP_PROXY only on
the test agent process. Never change its saved inform URL or authentication.
Only the configured source IP and exact upstream inform URL are accepted.
Packet contents and controller credentials are never logged or decrypted.
"""

import argparse
import datetime
import http.client
import http.server
import threading
import time
import urllib.parse


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bind', required=True, help='local address reachable from the test AP')
    parser.add_argument('--port', type=int, default=18080)
    parser.add_argument('--allow-client', required=True, help='test AP source IP')
    parser.add_argument('--upstream', required=True, help='real http://controller:port/inform URL')
    parser.add_argument('--fail-request', type=int, default=2, help='one-based request to fail; 0 disables injection')
    parser.add_argument('--duration', type=int, default=240, help='maximum lifetime in seconds')
    args = parser.parse_args()
    upstream = urllib.parse.urlsplit(args.upstream)
    if (upstream.scheme != 'http' or not upstream.hostname or upstream.path != '/inform'
            or upstream.username or upstream.password or upstream.query or upstream.fragment
            or args.fail_request < 0 or not 1 <= args.duration <= 600):
        parser.error('use an exact HTTP inform URL, nonnegative request number, and duration 1–600')
    count = 0
    lock = threading.Lock()

    def log(message):
        timestamp = datetime.datetime.now(datetime.timezone.utc).isoformat(timespec='seconds')
        print(f'{timestamp} {message}', flush=True)

    class Handler(http.server.BaseHTTPRequestHandler):
        protocol_version = 'HTTP/1.1'

        def log_message(self, *_):
            pass  # Do not log request headers, packet contents, or credentials.

        def reply(self, status, body=b'', content_type='application/x-binary'):
            self.send_response(status)
            self.send_header('Content-Type', content_type)
            self.send_header('Content-Length', str(len(body)))
            self.send_header('Connection', 'close')
            self.end_headers()
            self.wfile.write(body)
            self.close_connection = True

        def do_POST(self):
            nonlocal count
            self.connection.settimeout(15)
            if self.client_address[0] != args.allow_client or self.path != args.upstream:
                self.reply(403)
                return
            try:
                size = int(self.headers.get('Content-Length', '0'))
            except ValueError:
                self.reply(400)
                return
            if not 0 < size <= 16 * 1024 * 1024:
                self.reply(413)
                return
            packet = self.rfile.read(size)
            if len(packet) != size:
                self.reply(400)
                return
            with lock:
                count += 1
                request_number = count
            if request_number == args.fail_request:
                self.reply(503)
                log(f'request={request_number} injected_status=503')
                return
            connection = http.client.HTTPConnection(upstream.hostname, upstream.port or 80, timeout=15)
            try:
                connection.request('POST', '/inform', packet, {'Content-Type': 'application/x-binary'})
                response = connection.getresponse()
                body = response.read(16 * 1024 * 1024 + 1)
                if len(body) > 16 * 1024 * 1024:
                    raise ValueError('oversized controller response')
            except (OSError, http.client.HTTPException, ValueError) as error:
                # Log only the type, never a packet or credential-bearing URL.
                self.reply(502)
                log(f'request={request_number} upstream_error={type(error).__name__}')
                return
            finally:
                connection.close()
            self.reply(response.status, body, response.getheader('Content-Type', 'application/x-binary'))
            log(f'request={request_number} upstream_status={response.status}')

    server = http.server.ThreadingHTTPServer((args.bind, args.port), Handler)
    server.daemon_threads = True
    server.timeout = 1
    deadline = time.monotonic() + args.duration
    log(f'ready fail_request={args.fail_request} duration={args.duration}s')
    try:
        while time.monotonic() < deadline:
            server.handle_request()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()
        log(f'stopped requests={count}')


if __name__ == '__main__':
    main()
