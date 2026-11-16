// Real incidents of the kind each lesson rehearses.
//
// Every entry links to the report published by the people it happened to (or,
// for the two small companies, by the engineer who ran the system). Each fact
// below was read on that page on 16 November 2026. The wording is ours.

export const INCIDENTS = {
  preflight: [
    {
      who: "Gimme Bar",
      when: "October 2012",
      what: "A three-member MongoDB replica set had two of its voting members in the same availability zone. When that zone failed, two of the three voters went with it, and the one member left could not elect itself primary. The site went down with a member of the replica set still running.",
      why: "Nothing was broken before that day. The weakness was in where things had been placed, which is what a read-only check can find and a dashboard cannot.",
      link: "Read Sean Coates's account, “MongoDB Elections”",
      url: "https://seancoates.com/blogs/mongodb-elections",
    },
  ],
  hang: [
    {
      who: "GitHub",
      when: "21 October 2018",
      what: "The network link between two of GitHub's sites was lost for 43 seconds. Automated failover promoted databases at the other site in that time, and when the link returned each site held writes the other did not have. Service was degraded for 24 hours and 11 minutes while the data was reconciled.",
      why: "The partition was short. The cost came from two places both acting as primary, the same window this drill shows when the old primary wakes up.",
      link: "Read GitHub's “October 21 post-incident analysis”",
      url: "https://github.blog/news-insights/company-news/oct21-post-incident-analysis/",
    },
  ],
  crash: [
    {
      who: "Twilio",
      when: "18 July 2013",
      what: "The Redis primary holding account balances was restarted, read the wrong configuration file, and tried to recover from a file that did not exist. It came back with no balance data. Balances read as zero, so the billing system charged customers' cards again and again. Twilio reported that 1.4% of its customers were affected.",
      why: "The store did not report an error. It gave an answer, and the answer was wrong. That is the kind of failure this drill found: a valid key told it was not allowed.",
      link: "Read Twilio's “Billing Incident Post-Mortem”",
      url: "https://www.twilio.com/en-us/blog/company/communications/billing-incident-post-mortem-breakdown-analysis-and-root-cause-html",
    },
    {
      who: "Google",
      when: "14 December 2020",
      what: "The service that looks up user identities could not get current data for 47 minutes. Google's systems could not verify that requests were authenticated, and answered almost all authenticated traffic with 5xx errors.",
      why: "Worth comparing with the 403 above. Unable to check who the caller was, Google's systems said “error”, which tells a client to try again. They did not say “you are not allowed”.",
      link: "Read Google Cloud's incident report #20013",
      url: "https://status.cloud.google.com/incident/zall/20013",
    },
  ],
  pod: [
    {
      who: "Datadog",
      when: "8 March 2023",
      what: "An automatic security update restarted a system service on Datadog's hosts, and the restart deleted the network routes their containers depended on. Tens of thousands of nodes went offline within an hour, across five regions.",
      why: "This drill removes one gateway and finds almost nothing. Datadog's report is the reminder of what it does not cover: many nodes going at once, for a reason nobody had rehearsed.",
      link: "Read Datadog's “2023-03-08 Incident” report",
      url: "https://www.datadoghq.com/blog/2023-03-08-multiregion-infrastructure-connectivity-issue/",
    },
  ],
  rollout: [
    {
      who: "Slack",
      when: "22 February 2022",
      what: "A routine, staged rollout replaced a quarter of the fleet's Consul agents, as two earlier steps had done without trouble. This time the cache nodes that were replaced came back empty, the databases behind them took the extra reads, and the load grew faster than the cache misses did. Many users could not connect.",
      why: "The rollout was a planned change, done in careful stages, and had gone well twice. Planned restarts deserve a drill for the same reason unplanned ones do.",
      link: "Read “Slack's Incident on 2-22-22”",
      url: "https://slack.engineering/slacks-incident-on-2-22-22/",
    },
  ],
  "mongo-primary": [
    {
      who: "updown.io",
      when: "April 2019",
      what: "A failing disk made a MongoDB primary roughly 200 times slower. It still answered the replica set's heartbeats, so the healthy secondary beside it was never elected. The service was almost entirely down for hours, until someone stepped the primary down by hand at 8:56 in the morning.",
      why: "An election only happens when the members agree the primary is gone. This drill tests the clean cases, a kill and a step-down. The report is about the unclean one, where the primary is too slow to serve and too alive to replace.",
      link: "Read updown.io's “MongoDB SSD incident postmortem”",
      url: "https://updown.io/blog/mongodb-ssd-incident-postmortem",
    },
  ],
  "mongo-restore": [
    {
      who: "GitLab",
      when: "31 January 2017",
      what: "An engineer removed the database directory on the primary, believing it was the secondary. The regular backups turned out to have been failing without anyone seeing the alerts. The restore came from a snapshot taken six hours earlier, and copying it into place took around 18 hours. About 5,000 projects, 5,000 comments and 700 user accounts were lost.",
      why: "Nobody knew what the backup held or how long a restore would take until the day it was needed. Those are the two numbers this drill measures.",
      link: "Read GitLab's “Postmortem of database outage of January 31”",
      url: "https://about.gitlab.com/blog/postmortem-of-database-outage-of-january-31/",
    },
    {
      who: "Atlassian",
      when: "5 April 2022",
      what: "A maintenance script was given the wrong identifiers and deleted 883 sites belonging to 775 customers. The backups were sound, and Atlassian reported that it met its target for how much data could be lost. What it missed was the time: there was no automated way to restore that many sites, and some customers waited up to 14 days.",
      why: "Having a good backup and being able to restore it quickly are different abilities. Atlassian's own summary was that it missed its recovery time and met its recovery point.",
      link: "Read Atlassian's “Post-Incident Review on the April 2022 outage”",
      url: "https://www.atlassian.com/blog/how-we-build/post-incident-review-april-2022-outage",
    },
  ],
};
