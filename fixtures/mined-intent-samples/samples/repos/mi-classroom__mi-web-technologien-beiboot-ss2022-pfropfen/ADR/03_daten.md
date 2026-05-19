# Daten

## Status

accepted

## Context

Die Daten die auf der Website angezeigt werden befinden sich in einer JSON-Datei. Diese soll nicht Teil des Repositorys sein, damit der Inhalt vor fremdem Zugriff geschützt ist.
Um die Datei nicht mit ins Repository zu pushen wird der Dateiname in der .gitignore-Datei eingetragen.

Eine Möglichkeit ist es, die Datei auf einem externen Webserver anzubieten, von dem die Daten bei Bedarf übertragen werden.
Eine weitere Möglichkeit ist es einen File-Upload einzubauen, so dass jeder Anwender selbst im Besitz der Datei sein muss.
Eine Verschlüsselung der Datei ist als eine weitere Möglichkeit denkbar.
Wenn die Applikation als Docker Container angeboten wird, kann die json-Datei auch in das Docker Image integriert werden.


## Decision
Die Daten werden in ein Docker Image integriert, welches der Anwender über den offiziellen Docker Hub runterladen kann um
einen Container mit der Applikation zu starten. 

Das Repository beinhaltet die JSON-Datei nicht.





## Consequences
Auf dem System des Anwenders muss die Docker Software installiert sein. Er kann über den "docker run" Befehl einen Container
auf Basis des angebotenen Images starten.

Um die Website lokal ohne das Docker Image zu betreiben muss der Benutzer selbst im Besitz der JSON-Datei sein.
