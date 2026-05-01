# v-drawable-to-glb (CLI)

Small Go CLI that uploads a GTA V `.ydr` or `.ydd` plus optional `.ytd` to a **drawable → GLB** HTTP API and saves the returned `.glb`.

No third-party Go modules; uses only the standard library.

## Limits and API keys

Without an API key, usage is typically capped at **15 conversion attempts per client IP address per rolling 24-hour window**. Passing an API key raises that quota.

You can create a free API key and use Bearer authentication as documented at **[gtax.dev/api-keys](https://gtax.dev/api-keys)**.

Set the key with `-api-key` or the environment variable `V_DRAWABLE_TO_GLB_API_KEY`.

## API endpoint

The CLI always uses the hosted conversion service at **`https://public-drawable-to-glb.gtax.dev`**.

## Build

```bash
go build -o v-drawable-to-glb .
```

## Usage

```text
v-drawable-to-glb -i <file.ydr|file.ydd> [options]
```

### HTTP API (what the client sends)

The CLI posts `multipart/form-data` to:

- `POST {base}/convert/ydr-to-glb` for `.ydr` input (see service docs for `rotationX` / `rotationY` / `rotationZ`)
- `POST {base}/convert/ydd-to-glb` for `.ydd` input

Successful responses use `Content-Type: model/gltf-binary`. Errors are JSON: `{"error":"..."}`.

### Flags

| Flag | Description |
|------|-------------|
| `-i` | Input `.ydr` or `.ydd` (required). Extension selects the conversion path. |
| `-o` | Output `.glb` path. Default: input basename with `.glb`. |
| `-ytd` | Optional `.ytd` texture dictionary. |
| `-name` | Optional output filename stem (`name` form field). |
| `-lod` | Optional: `high`, `medium`, `low`, `verylow`. |
| `-drawable` | Optional YDD drawable name. |
| `-drawable-index` | Optional non-negative YDD index. Default `-1` = omit. |
| `-rotation-x` | Optional degrees about world **+X** (root node; combined order X→Y→Z). Default `0`. |
| `-rotation-y` | Optional degrees about world **+Y**. Default `0`. |
| `-rotation-z` | Optional degrees about world **+Z**. Default `0`. |
| `-api-key` | Bearer token, or env `V_DRAWABLE_TO_GLB_API_KEY`. |
| `-timeout` | HTTP client timeout (default: 30m). |

### Examples

From the `v-drawable-to-glb` directory, the **`example/`** folder contains a small YDR+YTD pair and a YDD+YTD pair you can pass through the CLI:

**YDR** — `prop_phone_ing_03.ydr` and `cellphone_badger.ytd`:

```bash
go build -o v-drawable-to-glb .
./v-drawable-to-glb -i example/prop_phone_ing_03.ydr -ytd example/cellphone_badger.ytd -name phone -lod high -rotation-y 90 -o example/phone.glb
```

**YDD** — `lowr_057_u.ydd` and `lowr_diff_057_a_uni.ytd` (first drawable in the dictionary):

```bash
./v-drawable-to-glb -i example/lowr_057_u.ydd -ytd example/lowr_diff_057_a_uni.ytd -name lowr -drawable-index 0 -o example/lowr.glb -rotation-x -90
```

With an API key (higher quota):

```bash
./v-drawable-to-glb -api-key "$V_DRAWABLE_TO_GLB_API_KEY" -i example/prop_phone_ing_03.ydr -ytd example/cellphone_badger.ytd -name phone -o example/phone.glb
```

On success, useful response headers are summarized on stderr; the GLB body is written to `-o`.
