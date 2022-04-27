# Bhojpur Action - Software Build & Release

The `Bhojpur Action` is a custom software *build* and *release* __automation engine__ based on
the [Bhojpur ISO](https://github.com/bhojpur/iso/) toolkit, which is pre-integrated with the
`GitHub` platform already. It is used across the *Bhojpur Consulting* organisation by the
[Bhojpur.NET Platform](https://github.com/bhojpur/platform) development teams for automation
purposes.

Please feel free to use it in your `GitHub` repository workflows. Also, you could contact
[Bhojpur Consulting](https://www.bhojpur-consulting.com) over phone/text or email or through
our [Global Support Centre](https://desk.bhojpur-consulting.com), if you have any queries
specifically related to this software tool.

## Building Packages

```yaml
- name: Build packages
  uses: bhojpur/action
  with:
    # tree: packages
    build: true
```
