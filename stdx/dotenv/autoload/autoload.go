package autoload

/*
	You can just read the .env file on import just by doing

		import _ "markdown.ninja/stdx/dotenv/autoload"

	And bob's your mother's brother
*/

import dotenv "markdown.ninja/stdx/dotenv"

func init() {
	dotenv.Load()
}
