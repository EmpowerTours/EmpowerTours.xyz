# EmpowerTours - Railway Deployment Checklist

## 1. Deploy to Railway

1. Go to [railway.app](https://railway.app) → Sign in with GitHub
2. **New Project** → **Deploy from GitHub Repo** → `EmpowerTours/EmpowerTours.xyz`
3. Select branch: **`api`**
4. Railway auto-detects the Dockerfile and builds

## 2. Add Volume (for SQLite persistence)

1. Click your service → **Settings** → **Volumes**
2. **Add Volume** → Mount path: `/data`
3. This ensures your database survives redeploys

## 3. Set Environment Variables

| Variable | Value | Required |
|----------|-------|----------|
| `DATABASE_PATH` | `/data/empowertours.db` | Yes |
| `PORT` | `8080` | Yes |
| `JWT_SECRET` | Random 64-char hex string | Yes |
| `BASE_URL` | `https://api.empowertours.xyz` | Yes |
| `STRIPE_SECRET_KEY` | `sk_live_...` from Stripe dashboard | For payments |
| `STRIPE_WEBHOOK_SECRET` | `whsec_...` from Stripe dashboard | For payments |
| `MONAD_RPC_URL` | `https://rpc.monad.xyz` | For blockchain |
| `APPLE_ISSUER_ID` | From App Store Connect → Keys | For iOS IAP |
| `APPLE_KEY_ID` | From App Store Connect → Keys | For iOS IAP |
| `APPLE_PRIVATE_KEY` | `.p8` file contents (with \n) | For iOS IAP |
| `APPLE_BUNDLE_ID` | `xyz.empowertours.mobile` | For iOS IAP |
| `GOOGLE_PLAY_CREDENTIALS_JSON` | Service account JSON | For Android IAP |

Generate a JWT secret: `openssl rand -hex 32`

## 4. Custom Domain

1. In Railway: **Settings** → **Networking** → **Custom Domain**
2. Add: `api.empowertours.xyz`
3. Railway gives you a CNAME target (e.g. `xxx.up.railway.app`)
4. In your DNS provider, add:
   - Type: `CNAME`
   - Name: `api`
   - Value: `xxx.up.railway.app` (from Railway)
5. If using Cloudflare: set proxy to **DNS only** (grey cloud) so Railway handles TLS

## 5. Stripe Webhook

1. Go to [dashboard.stripe.com/webhooks](https://dashboard.stripe.com/webhooks)
2. **Add endpoint**: `https://api.empowertours.xyz/webhooks/stripe`
3. Events to listen for: `payment_intent.succeeded`, `payment_intent.payment_failed`
4. Copy the **Signing secret** (`whsec_...`) → set as `STRIPE_WEBHOOK_SECRET` env var

## 6. Post-Deploy Checklist

- [ ] `curl https://api.empowertours.xyz/health` returns `{"data":{"status":"ok"}}`
- [ ] `curl https://api.empowertours.xyz/privacy` returns privacy policy page
- [ ] `curl https://api.empowertours.xyz/admin` returns admin panel
- [ ] Register a test account via admin panel
- [ ] Create a test experience via admin panel
- [ ] Verify mobile app connects (update API URL to production)

## 7. Update Mobile App for Production

In `src/config/api.ts`, the `PROD_API_BASE` is already set to `https://api.empowertours.xyz`. When you build with `eas build --profile production`, it will use the production URL automatically.
