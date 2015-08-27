# wiwi-mailporter

## Kurzanleitung

### Ordner anzeigen:

```
wiwi-mailporter list notes
```

```
wiwi-mailporter list exchange
```

### Übertragung von Notes INBOX zu Exchange INBOX:

```
wiwi-mailporter transfer -b 19-Aug-2015 INBOX INBOX
```

Der Parameter `-b` ist optional: das Datum, vor dem die Mails übertragen werden sollen (normalerweise der Tag der Umstellung).

### Logindaten löschen

Das Programm fragt beim ersten Aufruf die Logindaten für Notes und Exchange ab. Diese werden temporär zwischengespeichert (folgende Aufrufe verwenden sie wieder). Um sie zu löschen (nicht mehr nötig oder falsch eingegeben):

```
wiwi-mailporter clear
```
