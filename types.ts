export interface Project {
  id: string;
  name: string;
  category: string;
  image: string;
  url: string;
  description: string;
  features: string[];
  technologies: string[];
}

export interface Service {
  title: string;
  description: string;
  icon: any;
}

export enum Section {
  HERO = 'hero',
  SERVICES = 'services',
  PORTFOLIO = 'portfolio',
  CONTACT = 'contact',
}
