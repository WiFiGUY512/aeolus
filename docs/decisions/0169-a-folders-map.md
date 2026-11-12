# 0169. A folder's map

- Status: Proposed
- Date: 2026-10-15
- Proposed by: Griff: "add a folder based google maps page under system". Written up by Claude
- Refines: 0052, 0154, 0168

## Context

- **Aeolus knew what each folder holds, not where it is.** A site of many folders, or an org of many sites, had nothing to show which is where, or which is down where.
- **Griff likes Omnisight's site page:** an address finder, GPS, and a map.
- **Google's maps need a key, paid for past a free amount, and terms that forbid using their tiles any other way.** OpenStreetMap's tiles and its Nominatim address search need no key: only a name for the application asking, light use, and the map credited.
- **Griff wants addresses suggested as they are typed,** as Google Maps does. Nominatim's policy forbids that; Photon, OpenStreetMap's geocoder made for it, allows it in fair use.
- **The manager's pages load nothing from elsewhere** (their content security policy allows images and requests only from the manager). An image tag cannot carry the person's token.

## Decision

- **Where a folder or an AP is, is `system.place`:** `address`, `latitude` and `longitude`, in Locations, inherited as any setting is. A folder's APs are drawn at its place unless one is set on an AP itself.
  - **It is the manager's own,** as the DNS server's log (0154) and the switches (0168) are: no AP is sent it.
  - **A place is a latitude and a longitude set on one node.** One of each from different places is no place.
  - **A change to it, or to any of those, gives no AP a new config version.** It changes nothing an AP runs.
- **Each folder's System tab, and each AP's, starts with its map.**
  - **A folder's map shows it and all below it;** an AP's map shows its folder's, with the AP ringed.
  - **A folder is a square pin. An AP is a round pin, coloured as its dot in the tree is.** A folder or AP with no place of its own sits around the pin of the one it takes its place from, so that each is seen and none hides another.
  - **Pressing a pin shows what it is:** its status, address and coordinates, the folder it is drawn by, and a link that opens the place in Google Maps.
  - **The map is dragged to move, and wheeled or buttoned to zoom, a level at a time.** It opens fitted to its pins. Drawn again, as the page is every half minute, it stays as it was left; while a spot is picked, the page is not drawn again under it.
- **Someone who may change the folder or AP puts it on the map,** in one change, previewed and applied as any other, by any of:
  - its address, suggested as it is typed: from three letters, five places at most under the box, those near where the map looks first, picked by a click or by the arrows and Enter. Enter with none picked finds the address by Nominatim, one of up to five places then picked;
  - a click where it is, with *Put it where I click* on;
  - its latitude and longitude, typed.
- **The manager asks OpenStreetMap, not the page:**
  - **`GET /v1/map/tiles/{z}/{x}/{y}`** is a tile, for anyone signed in. The manager fetches it from tile.openstreetmap.org and keeps the latest 32 MB of tiles for a week. The page fetches each with its token and draws it on a canvas.
    - **The manager fetches as a person looking at maps would:** two tiles at once at most, 120 at first and four a second after, and a tile asked for twice while it comes is fetched once. Past that, the answer is 503 and the page asks again a while later.
    - **A tile the server cannot give is given as it was last kept,** however old.
  - **`GET /v1/map/find?q=`** is where an address may be. The manager asks Nominatim at most once a second, and keeps each answer a day.
  - **`GET /v1/map/suggest?q=&lat=&lon=`** is where an address being typed may be. The manager asks Photon (photon.komoot.io) at most four times a second, and keeps each answer a day; the page waits for a pause in the typing before it asks.
  - **Each names the manager as Aeolus.** The map credits OpenStreetMap's contributors.
- **`GET /v1/places?under=`** is the node and each folder and AP below it the caller may view, with its place and whether it is its own.

## Consequences

- **Which folder is where, and which of its APs are down there, is seen at once.**
- **The map needs the manager to reach the internet.** Without it, the pins are drawn on a blank map and an address cannot be found; a typed latitude and longitude still places one.
- **OpenStreetMap's maps, not Google's.** A Google map would need Griff's own API key; each pin links to Google Maps instead.
- **To come:** an Org-wide map of every site; floor plans; weather and other layers, as Omnisight has.
