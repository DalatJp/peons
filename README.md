# Peons

**Un petit bonhomme par session Claude Code, sur une île d'herbe qui flotte sur ton bureau.**

Tu lances trois, quatre, sept sessions Claude Code. Laquelle travaille ? Laquelle t'attend depuis dix minutes sans oser le dire ? Laquelle s'est endormie ? Laquelle est presque à court de contexte ?

Plus besoin de faire le tour des terminaux. Un coup d'œil sur l'île et tu sais : chaque bonhomme (le « clawd ») mime ce que fait **son** agent, en direct.

<p align="center">
  <img src="docs/img/scene.gif" alt="Cinq bonhommes sur la même île : un mineur, un qui brandit un panneau « ! », un écrasé par un piston, un content, un endormi" width="100%">
</p>

De gauche à droite : il creuse, il attend ta réponse, il compacte, il est content, il dort. Pas de fenêtre à ouvrir, pas de terminal à fouiller : l'île reste au-dessus des autres fenêtres, transparente, et tu peux la glisser où tu veux.

## Ce que chaque état veut dire

### Claude s'active

<table>
<tr>
<td align="center" width="33%"><img src="docs/img/mining.gif" alt="Le clawd creuse une roche" width="200"><br><b>Il creuse</b><br>Claude travaille. La roche se fend peu à peu (ici en accéléré), explose au bout d'une minute, puis une neuve tombe du ciel.</td>
<td align="center" width="33%"><img src="docs/img/alert.gif" alt="Le clawd brandit un panneau « ! »" width="200"><br><b>Panneau « ! »</b><br>Claude te demande une permission et attend ta réponse. C'est le seul qui a vraiment besoin de toi.</td>
<td align="center" width="33%"><img src="docs/img/compacting.gif" alt="Un piston écrase le clawd" width="200"><br><b>Écrasé par un piston</b><br>Claude compacte son contexte. Ça pèse, il le montre.</td>
</tr>
</table>

### Claude a fini, et tu le laisses seul

Le temps passe, l'humeur change. Tout se règle dans les réglages ; par défaut :

<table>
<tr>
<td align="center" width="33%"><img src="docs/img/content.gif" alt="Le clawd content, yeux fermés en sourire" width="200"><br><b>Content</b><br>Claude vient de finir une réponse. Ça dure 2 min.</td>
<td align="center" width="33%"><img src="docs/img/normal.gif" alt="Le clawd neutre" width="200"><br><b>Neutre</b><br>Rien à signaler (c'est aussi son état au démarrage). 5 min.</td>
<td align="center" width="33%"><img src="docs/img/triste.gif" alt="Le clawd triste" width="200"><br><b>Triste</b><br>Tu l'as oublié. Il boude 10 min, puis il s'endort.</td>
</tr>
<tr>
<td align="center" width="33%"><img src="docs/img/endormi.gif" alt="Le clawd dort, bulles de Z" width="200"><br><b>Endormi</b><br>Il dort jusqu'à ce que tu lui reparles.</td>
<td align="center" width="33%"><img src="docs/img/bench.gif" alt="Le clawd fait de la fonte sur un banc" width="200"><br><b>Au banc</b><br>Endormi depuis longtemps, il se réveille 20 s toutes les 15 min pour soulever de la fonte. Puis il se rendort.</td>
<td></td>
</tr>
</table>

### Il arrive, il part

<table>
<tr>
<td align="center" width="50%"><img src="docs/img/portail.gif" alt="Un clawd tombe d'un portail bleu" width="240"><br><b>Nouvelle session</b><br>Un portail s'ouvre, le clawd en tombe. Bienvenue.</td>
<td align="center" width="50%"><img src="docs/img/snap.gif" alt="Le clawd se désagrège en poussière" width="240"><br><b>Session terminée</b><br>Il se désagrège en poussière, façon Thanos, et les autres se resserrent. Même chose si tu fermes le terminal.</td>
</tr>
</table>

## Et les sous-agents ?

Quand une session lance des sous-agents (l'outil Agent), une petite particule flotte au-dessus de la tête du clawd pour chacun d'eux. Tu vois d'un coup d'œil combien bossent pour lui, sans que son corps change.

<p align="center">
  <img src="docs/img/sous-agents.gif" alt="Un mineur avec trois particules au-dessus de la tête, un clawd content avec deux particules vertes" width="70%">
</p>

- **Blanche, grande** : le sous-agent utilise des outils.
- **Blanche, petite** : il est vivant mais muet depuis plus de 10 s.
- **Verte** : la session principale a fini son tour et attend ses sous-agents.

Un sous-agent sans nouvelles depuis 2 minutes est oublié (au cas où il n'aurait jamais dit au revoir).

## Au survol, les détails

Passe la souris sur un bonhomme : une petite étiquette (Windows) apparaît avec

- le titre de la session (celui de l'onglet de ton terminal), et le dossier du projet dessous ;
- son état et depuis combien de temps, et l'outil en cours (`Bash`, `Edit`...) ;
- une **barre de contexte** : elle se remplit, et vire du vert au rouge à l'approche de la limite, avec le nombre de tokens restants. C'est une estimation, mais bien assez bonne pour savoir qui va bientôt avoir besoin d'un `/compact`.

## Les réglages

Un double-clic sur l'île (ou « Réglages » dans le menu de l'icône de la zone de notification) ouvre une vraie fenêtre de réglages :

- brancher ou débrancher le hook dans Claude Code ;
- la taille des clawd (de 1× à 8×), celle de l'étiquette de survol (de 1× à 4×, à part) et celle de l'île, qui grandit toute seule avec le nombre de bonhommes (8 affichés au plus), ou que tu peux fixer à la main, ou carrément cacher ;
- la durée des humeurs : content, neutre, triste ;
- « toujours afficher », même quand aucune session n'est ouverte (sinon l'île se cache) ;
- la liste des sessions et le détail de celle que tu as choisie.

## Installation

Windows uniquement : l'interface parle directement à Win32, sans bibliothèque tierce. Linux et macOS ne sont plus pris en charge (ils s'appuyaient sur SDL3, retiré).

```powershell
.\build.ps1
```

Ça lance les tests, puis produit `peons.exe` (un seul fichier, sans DLL). **Double-clic** sur `peons.exe`, puis **Brancher** dans la fenêtre de réglages. C'est tout : à partir de là, les sessions Claude Code font apparaître leurs bonhommes toutes seules, et l'application se lance d'elle-même au besoin.

### Ou en plugin Claude Code

Sans compiler, sans PowerShell, sans « Brancher » : le plugin branche les hooks et télécharge `peons.exe` depuis les releases GitHub (avec `curl.exe`, fourni par Windows).

```
claude plugin marketplace add aero-md/peons
claude plugin install peons@peons
```

Ne l'utilisez pas en même temps que « Brancher » (chaque événement serait envoyé deux fois) : débranchez d'abord (`peons uninstall`). `peons.exe` est conservé dans le dossier de données du plugin et mis à jour quand une release plus récente existe.

Un clic sur un bonhomme met au premier plan le terminal de sa session (la fenêtre, pas l'onglet) et le fait clignoter.

Tout le reste (commandes, fonctionnement du hook, architecture, détails d'implémentation) est dans [`docs/technique.md`](docs/technique.md).

## D'où ça vient

Peons est **inspiré de [Notchi](https://github.com/sk-ruban/notchi)**, l'app macOS de sk-ruban où une petite mascotte vit dans l'encoche du Mac et réagit à Claude Code. L'idée de départ, c'est la sienne ; ici, un bonhomme par session, sur une île, dans une fenêtre transparente.

Licences, attributions et origine des images et de la police : voir [`LICENSE`](LICENSE) et [`UPSTREAM.md`](UPSTREAM.md).
