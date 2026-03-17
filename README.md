# fog

**fog** is a multi-language bundle runner. Pack multiple source files — across any mix of languages — into a single `.fog` file and run them with one command.

## Bundle Format

A `.fog` file is plain text with `[filename.ext]` section headers.

```
[greet.py]
print("Hello from Python!")

[app.js]
console.log("Hello from JavaScript!")

[run.sh]
echo "Hello from Shell!"
```

## Metadata Directives

```
[@name=my-app]
[@version=1.0.0]
[@desc=My application]
[@entrypoint=main.py]
[@parallel]
[@pipeline=step1.py,step2.js,report.sh]
[@stop-on-fail]
```

## File-level Attributes

```
[worker.py tags=background,optional desc=Background worker]
...

[heavy.js disabled]
...

[process.sh after=setup.py args=--verbose]
...
```

## Cross-Language Bridge

When `--no-bridge` is not set, fog injects helpers into Python, JavaScript, Ruby, and Shell files so they can share data at runtime:

**Python:**
```python
fog_export("RESULT", "42")   # write to bridge + os.environ
val = fog_get("RESULT")      # read from bridge
```

**JavaScript:**
```javascript
fog.export("RESULT", "42");
const val = fog.get("RESULT");
```

**Shell:**
```bash
fog_export RESULT "42"
val=$(fog_get RESULT)
```

## Commands

```sh
fog run bundle.fog                         # run entrypoint
fog run bundle.fog --entry worker.py       # override entrypoint
fog run bundle.fog --tag background        # run tagged files
fog run bundle.fog --parallel              # run all in parallel
fog run bundle.fog --pipeline a.py,b.js   # pipe stdout to stdin
fog run bundle.fog --watch                 # re-run on change
fog run bundle.fog --env KEY=VAL           # inject env var
fog run bundle.fog -- --port 8080          # pass args to script
fog run bundle.fog --verbose               # show execution details
fog run bundle.fog --dry-run               # preview only
fog run bundle.fog --keep-temp             # keep temp workspace

fog pack src/ -o app.fog                   # pack directory
fog pack main.py utils.py -o app.fog       # pack specific files
fog pack src/ --entry main.py -o app.fog   # with entrypoint
fog pack src/ --parallel -o app.fog        # parallel bundle
fog pack src/ --strip-empty -o app.fog     # skip empty files

fog extract app.fog                        # extract to current dir
fog extract app.fog -o ./output            # extract to directory
fog extract app.fog --file main.py         # extract specific file
fog extract app.fog --flat                 # ignore subdirs

fog inspect app.fog                        # show bundle info
fog inspect app.fog --verbose              # show tags and env
fog validate app.fog                       # validate bundle
fog diff a.fog b.fog                       # diff two bundles

fog new myapp python                       # create from template
fog new myapp web                          # HTML+CSS+JS bundle
fog new myapp fullstack                    # Python server + frontend
fog new myapp pipeline                     # multi-lang data pipeline
fog new myapp polyglot                     # mixed-language parallel

fog langs                                  # list supported languages
```

## Pipeline Mode

Stdout of each step is piped as stdin to the next:

```
[@pipeline=generate.py,process.js,report.sh]

[generate.py]
import json
print(json.dumps([1, 2, 3, 4, 5]))

[process.js]
let d = ''; process.stdin.on('data', c => d += c);
process.stdin.on('end', () => {
    const nums = JSON.parse(d);
    process.stdout.write(String(nums.reduce((a, b) => a + b, 0)));
});

[report.sh]
read n; echo "Sum is: $n"
```

## Parallel Mode

```
[@parallel]

[fetch.py]
print("fetching data...")

[render.js]
console.log("rendering...")

[notify.sh]
echo "notifying..."
```

## Templates

| Template    | Description                              |
|-------------|------------------------------------------|
| `python`    | Python multi-file starter                |
| `web`       | HTML + CSS + JavaScript                  |
| `fullstack` | Python HTTP server + HTML/CSS/JS client  |
| `pipeline`  | Python to JavaScript to Shell pipeline   |
| `polyglot`  | Python + JS + Shell in parallel          |

## Supported Languages

35+ languages including Python, JavaScript, TypeScript, Go, Rust, Ruby, Shell, PHP, Lua, Java, C, C++, R, Perl, Swift, Dart, Elixir, Julia, Haskell, Kotlin, Scala, Nim, Zig, C#, PowerShell, OCaml, Clojure, and data/asset types (HTML, CSS, JSON, YAML, SQL, Markdown, etc).
