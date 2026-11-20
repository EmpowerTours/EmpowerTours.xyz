<?php
// Mail relay for the EmpowerTours API on Railway (Railway Hobby blocks outbound SMTP).
// Accepts only HMAC-signed JSON from the API; always sends From admin@empowertours.xyz.
header('Content-Type: application/json');
header('Cache-Control: no-store');
function fail($code, $msg) { http_response_code($code); echo json_encode(['error' => $msg]); exit; }
if ($_SERVER['REQUEST_METHOD'] !== 'POST') fail(405, 'method');
$secret = @trim(file_get_contents('/home/empomeno/.et-mail-relay-secret'));
if (!$secret || strlen($secret) < 32) fail(500, 'not configured');
$body = file_get_contents('php://input', false, null, 0, 200000);
$ts = $_SERVER['HTTP_X_RELAY_TS'] ?? '';
$sig = $_SERVER['HTTP_X_RELAY_SIG'] ?? '';
if (!ctype_digit($ts) || abs(time() - (int)$ts) > 300) fail(401, 'stale');
if (!hash_equals(hash_hmac('sha256', $ts . "\n" . $body, $secret), $sig)) fail(401, 'signature');
// Replay guard: each signature is accepted once within the window.
$seen = '/home/empomeno/.et-mail-relay-seen';
$now = time(); $keep = [];
foreach (@file($seen, FILE_IGNORE_NEW_LINES) ?: [] as $line) {
  [$t, $s] = array_pad(explode(' ', $line, 2), 2, '');
  if ($now - (int)$t <= 600) { if ($s === $sig) fail(409, 'replay'); $keep[] = $line; }
}
$keep[] = $now . ' ' . $sig;
file_put_contents($seen, implode("\n", $keep) . "\n", LOCK_EX);
$m = json_decode($body, true);
if (!is_array($m)) fail(400, 'json');
$to = $m['to'] ?? ''; $reply = $m['reply_to'] ?? ''; $name = $m['from_name'] ?? 'EmpowerTours';
$subject = $m['subject'] ?? ''; $text = $m['body'] ?? '';
foreach ([$to, $reply, $name, $subject] as $h) if (preg_match('/[\r\n]/', $h)) fail(400, 'header');
if (!filter_var($to, FILTER_VALIDATE_EMAIL)) fail(400, 'to');
if ($reply !== '' && !filter_var($reply, FILTER_VALIDATE_EMAIL)) fail(400, 'reply_to');
$name = preg_replace('/[^\p{L}\p{N} .\-]/u', '', $name);
$from = 'admin@empowertours.xyz';
$headers = 'From: ' . mb_encode_mimeheader($name, 'UTF-8') . " <$from>\r\n"
  . ($reply !== '' ? "Reply-To: $reply\r\n" : '')
  . "MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit";
$ok = mail($to, mb_encode_mimeheader($subject, 'UTF-8'), str_replace("\n", "\r\n", $text), $headers, '-f' . $from);
if (!$ok) fail(502, 'mail');
echo json_encode(['ok' => true]);
