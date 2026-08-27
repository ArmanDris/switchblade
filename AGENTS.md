This GO project, `switchblade` applies a specified theme to both ghostty and
helix. To do this the project must first ensure the theme is inside the
respective Ghostty and Helix config, so it must first check, and if it does not 
exist it must copy the files in.

Once the theme is confirmed to exist in both Helix and Ghostty it must update
the config of both to the new theme. Then it must reload the ghostty config so
the update is applied.
