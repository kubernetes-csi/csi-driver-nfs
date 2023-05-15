import argparse
import datetime
import re
from collections import defaultdict
import subprocess
import shutil
from dateutil.relativedelta import relativedelta

# Check that the `gh` command is in the path
def check_gh_command():
    if not shutil.which('gh'):
        print("Error: The `gh` command is not available in the PATH.")
        print("Please install the GitHub CLI (https://cli.github.com/) and try again.")
        exit(1)

# humanize duration outputs
def duration_ago(dt):
    delta = relativedelta(datetime.datetime.now(), dt)
    if delta.years > 0:
        return f"{delta.years} year{'s' if delta.years > 1 else ''} ago"
    elif delta.months > 0:
        return f"{delta.months} month{'s' if delta.months > 1 else ''} ago"
    elif delta.days > 0:
        return f"{delta.days} day{'s' if delta.days > 1 else ''} ago"
    elif delta.hours > 0:
        return f"{delta.hours} hour{'s' if delta.hours > 1 else ''} ago"
    elif delta.minutes > 0:
        return f"{delta.minutes} minute{'s' if delta.minutes > 1 else ''} ago"
    else:
        return "just now"

def parse_version(version):
    # version pattern
    pattern = r"v(\d+)\.(\d+)\.(\d+)"
    match = re.match(pattern, version)
    if match:
        major, minor, patch =  map(int, match.groups())
        return (major, minor, patch)

# Calculate the end of life date for a minor release version 
# according to : https://kubernetes-csi.github.io/docs/project-policies.html#support
def end_of_life_grouped_versions(versions):
    supported_versions = []
    # Prepare dates for later calculation
    now          = datetime.datetime.now()
    one_year     = datetime.timedelta(days=365)
    three_months = datetime.timedelta(days=90)

    # get the newer versions on top
    sorted_versions_list = sorted(versions.items(), key=lambda x: x[0], reverse=True)
    # structure example :
    #  [((3, 5), [('v3.5.0', datetime.datetime(2023, 4, 27, 22, 28, 6))]),
    #   ((3, 4),
    #   [('v3.4.1', datetime.datetime(2023, 4, 5, 17, 41, 15)),
    #    ('v3.4.0', datetime.datetime(2022, 12, 27, 23, 43, 41))])]
    latest = sorted_versions_list.pop(0)

    # the latest version is always supported no matter the release date
    supported_versions.append(latest[1][-1])

    for v in sorted_versions_list:
        first_release = v[1][-1]
        last_release  = v[1][0]
        # if the release is less than a year old we support the latest patch version
        if now - first_release[1] < one_year:
            supported_versions.append(last_release)
        # if the main release is older than a year and has a recent path, this is supported
        elif now - last_release[1] < three_months:
            supported_versions.append(last_release)
    return supported_versions

def get_release_docker_image(repo, version):
    output = subprocess.check_output(['gh', 'release', '-R', repo, 'view', version], text=True)
    match = re.search(r"`docker pull (.*)`", output)
    docker_image = match.group(1)
    return((version, docker_image))

def get_versions_from_releases(repo):
    # Run the `gh release` command to get the release list
    output = subprocess.check_output(['gh', 'release', '-R', repo, 'list'], text=True)
    # Parse the output and group by major and minor version numbers
    versions = defaultdict(lambda: [])
    for line in output.strip().split('\n'):
        parts = line.split('\t')
        # pprint.pprint(parts)
        version = parts[0]
        parsed_version = parse_version(version)
        if parsed_version is None:
            continue
        major, minor, patch = parsed_version

        published = datetime.datetime.strptime(parts[3], '%Y-%m-%dT%H:%M:%SZ')
        versions[(major, minor)].append((version, published))
    return(versions)


def main():
    # Argument parser
    parser = argparse.ArgumentParser(description='Get the currently supported versions for a GitHub repository.')
    parser.add_argument(
                        '--repo',
                        '-R', required=True,
                        action='append', dest='repos',
                        help='''The name of the repository in the format owner/repo. You can specify multiple -R repo to query multiple repositories e.g.:\n
                                python -R kubernetes-csi/external-attacher -R kubernetes-csi/external-provisioner -R kubernetes-csi/external-resizer -R kubernetes-csi/external-snapshotter -R kubernetes-csi/livenessprobe -R kubernetes-csi/node-driver-registrar -R kubernetes-csi/external-health-monitor'''
                        )
    parser.add_argument('--display', '-d', action='store_true', help='(default) Display EOL versions with their dates', default=True)
    parser.add_argument('--doc', '-D', action='store_true', help='Helper to https://kubernetes-csi.github.io/docs/ that prints Docker image for each EOL version')

    args = parser.parse_args()

    # Verify pre-reqs
    check_gh_command()

    # Process all repos
    for repo in args.repos:
        versions = get_versions_from_releases(repo)
        eol_versions = end_of_life_grouped_versions(versions)

        if args.display:
            print(f"Supported versions with release date and age of `{repo}`:\n")
            for version in eol_versions:
                print(f"{version[0]}\t{version[1].strftime('%Y-%m-%d')}\t{duration_ago(version[1])}")

        # TODO : generate proper doc ouput for the tables of: https://kubernetes-csi.github.io/docs/sidecar-containers.html
        if args.doc:
            print("\nSupported Versions with docker images for each end of life version:\n")
            for version in eol_versions:
                _, image = get_release_docker_image(args.repo, version[0])
                print(f"{version[0]}\t{image}")
        print()

if __name__ == '__main__':
    main()