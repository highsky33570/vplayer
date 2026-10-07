# Admin (gin-vue-admin)

Client choice: [gin-vue-admin](https://www.gin-vue-admin.com/)

## Setup

Clone into this folder (or sibling) when ready:

```bash
# example — use the official GVA repo version your team picks
git clone https://github.com/flipped-aurora/gin-vue-admin.git .
```

Wire GVA to the same MySQL (`vplayer`) and extend with video/CMS menus that call:

- upload to R2
- enqueue HLS transcode
- manage categories / ads / users

Do **not** add scrapers that pull third-party catalogs (e.g. olehdtv) for re-hosting.
