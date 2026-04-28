# Finnish Source Batch 2026-04-28

Collected on 2026-04-28 for parser-eval draft expansion.

Purpose:

- add more Finnish source variety before the first larger manual gold pass
- bias selection toward compounds, possessives, case-rich nouns, and long derived forms

Source handling:

- YLE and Helsingin Sanomat are kept here as provenance links plus short notes
- draft case text may normalize a headline or snippet into a short sentence for annotation
- Finnish Wikipedia text is reusable under CC BY-SA 4.0 with attribution to the linked page

## YLE

- `fi-0002`
  - source: Yle front page collected 2026-04-28
  - url: https://yle.fi/
  - title: Pankit eivat anna lainaa Kittilan syrjakylille, joten kunta osti nipun asuntoja eurolla
  - parser targets: `syrjakylille`, `asuntoja`, `eurolla`

- `fi-0003`
  - source: Yle front page collected 2026-04-28
  - url: https://yle.fi/
  - title: Uudenlaisen lastensuojelulaitos tehdaan sellaiseksi, ettei sielta enaa noin vain karata
  - parser targets: `lastensuojelulaitosta`, `uudenlaista`

- `fi-0004`
  - source: Yle front page collected 2026-04-28
  - url: https://yle.fi/
  - title: Muotoiluasiantuntija: Suomessa vallitsee designin nollatila, jota Iittalan Pokemon-yhteistyo kuvastaa
  - parser targets: `nollatila`, `Pokemon-yhteistyo`

- `fi-0005`
  - source: Yle topic page collected 2026-04-28
  - url: https://yle.fi/t/18-56439/fi
  - title: Pankkiautomaatti voi jaada, vaikka pankki panisi lapun luukulle
  - parser targets: `pankkiautomaatti`, `luukulle`

- `fi-0006`
  - source: Yle topic page collected 2026-04-28
  - url: https://yle.fi/t/18-220006/fi
  - title: Kajaani testaa ensimmaisena kuntana robottia, jolla voi teettaa lumityot, ruohonleikkuun ja lehtien puhaltamisen
  - parser targets: `lumityot`, `ruohonleikkuun`, `puhaltamisen`

## Helsingin Sanomat

- `fi-0007`
  - source: Helsingin Sanomat
  - url: https://kampanjat.hs.fi/climatefont/index-fi.html
  - title: Ilmastofontti
  - parser targets: `Ilmastofontti`, `muunneltava`, `OpenType-fontti`

- `fi-0008`
  - source: Helsingin Sanomat
  - url: https://kampanjat.hs.fi/climatefont/index-fi.html
  - title: Ilmainen fontti, joka konkretisoi ilmastonmuutoksen vaikutuksen
  - parser targets: `muotoilu`, `jaameren`, `dataan`

- `fi-0009`
  - source: Helsingin Sanomat
  - url: https://kampanjat.hs.fi/tutustu/hs-teema/
  - title: HS Teema - syvenny ajankohtaisiin aiheisiin
  - parser targets: `tietopaketti`, `aiheeseensa`

## Finnish Wikipedia

- `fi-0010`
  - source: Finnish Wikipedia
  - url: https://fi.wikipedia.org/wiki/Ilmastonmuutos
  - title: Ilmastonmuutos
  - parser targets: `Ilmastonmuutos`, `muutoksia`, `saailmioissa`
  - license: CC BY-SA 4.0

- `fi-0011`
  - source: Finnish Wikipedia
  - url: https://fi.wikipedia.org/wiki/Hyvinvointialue
  - title: Suomen hyvinvointialueet
  - parser targets: `Hyvinvointialueet`, `terveydenhuollon`, `toimeenpanolailla`
  - license: CC BY-SA 4.0

- `fi-0012`
  - source: Finnish Wikipedia
  - url: https://fi.wikipedia.org/wiki/Suomenlahti
  - title: Suomenlahti
  - parser targets: `Suomenlahden`, `valuma-alueella`, `ihmista`
  - license: CC BY-SA 4.0
